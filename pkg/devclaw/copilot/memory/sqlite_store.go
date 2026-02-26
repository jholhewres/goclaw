// Package memory – sqlite_store.go implements a SQLite-backed memory store with
// FTS5 (BM25 ranking) and in-process vector search (cosine similarity).
// Embeddings are stored as JSON-encoded float32 arrays in the chunks table.
// This avoids the need for the sqlite-vec extension while still providing
// hybrid semantic + keyword search.
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver with FTS5 support.
)

// SQLiteStore provides persistent memory storage with hybrid search.
type SQLiteStore struct {
	db       *sql.DB
	embedder EmbeddingProvider
	logger   *slog.Logger

	// ftsAvailable indicates whether FTS5 is available for full-text search.
	// When false, search falls back to LIKE queries (slower but functional).
	ftsAvailable bool

	// vectorCache holds all chunk embeddings in memory for fast cosine search.
	// Refreshed on index operations.
	vectorCacheMu sync.RWMutex
	vectorCache   []vectorCacheEntry
}

// vectorCacheEntry holds a chunk embedding for in-memory vector search.
type vectorCacheEntry struct {
	chunkID   int64
	fileID    string
	text      string
	embedding []float32
}

// SearchResult represents a single search hit with score.
type SearchResult struct {
	FileID string
	Text   string
	Score  float64
}

// NewSQLiteStore opens or creates a SQLite memory database with FTS5.
func NewSQLiteStore(dbPath string, embedder EmbeddingProvider, logger *slog.Logger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &SQLiteStore{
		db:       db,
		embedder: embedder,
		logger:   logger,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Load vector cache into memory.
	if err := store.refreshVectorCache(); err != nil {
		logger.Warn("failed to load vector cache", "error", err)
	}

	return store, nil
}

// initSchema creates the required tables and indices.
func (s *SQLiteStore) initSchema() error {
	// Core tables — always created.
	coreSchema := `
		CREATE TABLE IF NOT EXISTS files (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id   TEXT UNIQUE NOT NULL,
			hash      TEXT NOT NULL,
			indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS chunks (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id    TEXT NOT NULL,
			chunk_idx  INTEGER NOT NULL,
			text       TEXT NOT NULL,
			hash       TEXT NOT NULL,
			embedding  TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(file_id, chunk_idx)
		);

		CREATE TABLE IF NOT EXISTS embedding_cache (
			text_hash TEXT NOT NULL,
			provider  TEXT NOT NULL,
			model     TEXT NOT NULL,
			embedding TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (text_hash, provider, model)
		);
	`

	if _, err := s.db.Exec(coreSchema); err != nil {
		return err
	}

	// FTS5 full-text search — optional. Some SQLite builds don't include FTS5.
	// When unavailable, memory search falls back to LIKE queries (slower but functional).
	ftsSchema := `
		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			text,
			content='chunks',
			content_rowid='id',
			tokenize='porter unicode61'
		);

		CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
			INSERT INTO chunks_fts(rowid, text) VALUES (new.id, new.text);
		END;

		CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
			INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', old.id, old.text);
		END;

		CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
			INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', old.id, old.text);
			INSERT INTO chunks_fts(rowid, text) VALUES (new.id, new.text);
		END;
	`
	if _, err := s.db.Exec(ftsSchema); err != nil {
		// FTS5 not available — mark it and continue. Search will use LIKE fallback.
		s.ftsAvailable = false
		slog.Warn("FTS5 not available, falling back to LIKE search", "error", err.Error())
	} else {
		s.ftsAvailable = true
	}

	return nil
}

// IndexChunks indexes a set of chunks for a file. Uses delta sync: only
// re-embeds chunks whose hash has changed.
func (s *SQLiteStore) IndexChunks(ctx context.Context, fileID string, chunks []Chunk, fileHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if file is already indexed with same hash.
	var existingHash string
	err = tx.QueryRow("SELECT hash FROM files WHERE file_id = ?", fileID).Scan(&existingHash)
	if err == nil && existingHash == fileHash {
		return nil // File unchanged.
	}

	// Upsert file record.
	_, err = tx.Exec(`
		INSERT INTO files (file_id, hash, indexed_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(file_id) DO UPDATE SET hash = excluded.hash, indexed_at = CURRENT_TIMESTAMP
	`, fileID, fileHash)
	if err != nil {
		return err
	}

	// Get existing chunk hashes to identify what changed.
	existingChunks := make(map[string]string) // chunk_hash → embedding (JSON)
	rows, err := tx.Query("SELECT hash, embedding FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		if rows != nil {
			rows.Close()
		}
		// Non-fatal: treat all chunks as new.
		s.logger.Debug("could not read existing chunks", "file", fileID, "error", err)
	} else {
		for rows.Next() {
			var h string
			var emb sql.NullString
			if err := rows.Scan(&h, &emb); err == nil {
				if emb.Valid {
					existingChunks[h] = emb.String
				} else {
					existingChunks[h] = ""
				}
			}
		}
		rows.Close()
	}

	// Delete old chunks for this file.
	_, _ = tx.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)

	// Find chunks that need new embeddings.
	var textsToEmbed []string
	var embedIndices []int
	for i, chunk := range chunks {
		if _, ok := existingChunks[chunk.Hash]; !ok {
			textsToEmbed = append(textsToEmbed, chunk.Text)
			embedIndices = append(embedIndices, i)
		}
	}

	// Generate embeddings for new/changed chunks.
	var newEmbeddings [][]float32
	if len(textsToEmbed) > 0 && s.embedder.Name() != "none" {
		// Check embedding cache first.
		newEmbeddings = make([][]float32, len(textsToEmbed))
		var uncachedTexts []string
		var uncachedIndices []int

		for i, text := range textsToEmbed {
			cached := s.getEmbeddingCache(text)
			if cached != nil {
				newEmbeddings[i] = cached
			} else {
				uncachedTexts = append(uncachedTexts, text)
				uncachedIndices = append(uncachedIndices, i)
			}
		}

		// Embed uncached texts.
		if len(uncachedTexts) > 0 {
			embeddings, err := s.embedder.Embed(ctx, uncachedTexts)
			if err != nil {
				s.logger.Warn("embedding generation failed, indexing without vectors", "error", err)
			} else {
				for i, emb := range embeddings {
					idx := uncachedIndices[i]
					newEmbeddings[idx] = emb
					s.setEmbeddingCache(uncachedTexts[i], emb)
				}
			}
		}
	}

	// Insert chunks.
	stmt, err := tx.Prepare(`
		INSERT INTO chunks (file_id, chunk_idx, text, hash, embedding)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	newEmbedIdx := 0
	for i, chunk := range chunks {
		var embJSON sql.NullString

		// Try to reuse existing embedding.
		if existing, ok := existingChunks[chunk.Hash]; ok && existing != "" {
			embJSON = sql.NullString{String: existing, Valid: true}
		} else if newEmbeddings != nil {
			// Find the new embedding for this chunk.
			for j, idx := range embedIndices {
				if idx == i && j < len(newEmbeddings) && newEmbeddings[j] != nil {
					data, _ := json.Marshal(newEmbeddings[j])
					embJSON = sql.NullString{String: string(data), Valid: true}
					newEmbedIdx++
					break
				}
			}
		}

		_, err := stmt.Exec(chunk.FileID, chunk.Index, chunk.Text, chunk.Hash, embJSON)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Refresh vector cache.
	return s.refreshVectorCache()
}

// SearchBM25 performs a keyword search using FTS5 BM25 ranking.
// Falls back to LIKE-based search when FTS5 is not available.
// Applies query expansion to handle conversational queries.
func (s *SQLiteStore) SearchBM25(query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	// If FTS5 is not available, use LIKE fallback with expanded keywords.
	if !s.ftsAvailable {
		return s.searchLikeFallback(query, maxResults)
	}

	// Try phrase search first.
	safeQuery := sanitizeFTS5Query(query)
	if safeQuery == "" {
		return nil, nil
	}

	results, err := s.ftsQuery(safeQuery, maxResults)
	if err == nil && len(results) >= maxResults/2 {
		return results, nil
	}

	// Expand query: extract keywords and search with OR.
	keywords := extractKeywords(query)
	if len(keywords) > 0 {
		expandedQuery := expandQueryForFTS(keywords)
		if expandedQuery != "" && expandedQuery != safeQuery {
			moreResults, err := s.ftsQuery(expandedQuery, maxResults)
			if err == nil {
				results = mergeSearchResults(results, moreResults, maxResults*2)
			}
		}
	}

	return results, nil
}

// ftsQuery runs a single FTS5 query and returns ranked results.
func (s *SQLiteStore) ftsQuery(ftsQuery string, maxResults int) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT c.file_id, c.text, rank
		FROM chunks_fts
		JOIN chunks c ON c.id = chunks_fts.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, maxResults*2)
	if err != nil {
		return s.searchLikeFallback(ftsQuery, maxResults)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var rank float64
		if err := rows.Scan(&r.FileID, &r.Text, &rank); err != nil {
			continue
		}
		r.Score = 1.0 / (1.0 + math.Abs(rank))
		results = append(results, r)
	}

	return results, nil
}

// extractKeywords extracts meaningful keywords from a conversational query
// by removing stop words and short tokens.
func extractKeywords(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}*")
		if len(w) < 3 || stopWords[w] {
			continue
		}
		keywords = append(keywords, w)
	}
	return keywords
}

// expandQueryForFTS converts extracted keywords into an FTS5 OR query.
func expandQueryForFTS(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	var parts []string
	for _, kw := range keywords {
		s := sanitizeFTS5Query(kw)
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " OR ")
}

// mergeSearchResults deduplicates and merges two result sets.
func mergeSearchResults(a, b []SearchResult, maxResults int) []SearchResult {
	seen := make(map[string]bool)
	var merged []SearchResult
	for _, r := range a {
		key := r.FileID + "|" + r.Text
		if !seen[key] {
			seen[key] = true
			merged = append(merged, r)
		}
	}
	for _, r := range b {
		key := r.FileID + "|" + r.Text
		if !seen[key] {
			seen[key] = true
			merged = append(merged, r)
		}
	}
	if len(merged) > maxResults {
		merged = merged[:maxResults]
	}
	return merged
}

// stopWords are common words filtered out during keyword extraction.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true,
	"not": true, "you": true, "all": true, "can": true, "had": true,
	"her": true, "was": true, "one": true, "our": true, "out": true,
	"has": true, "its": true, "let": true, "may": true, "who": true,
	"did": true, "get": true, "got": true, "him": true, "his": true,
	"how": true, "man": true, "new": true, "now": true, "old": true,
	"see": true, "way": true, "day": true, "too": true, "use": true,
	"that": true, "with": true, "have": true, "this": true, "will": true,
	"your": true, "from": true, "they": true, "been": true, "said": true,
	"each": true, "which": true, "their": true, "what": true, "about": true,
	"would": true, "there": true, "when": true, "make": true, "like": true,
	"time": true, "just": true, "know": true, "take": true, "come": true,
	"could": true, "than": true, "look": true, "only": true, "into": true,
	"over": true, "such": true, "also": true, "back": true, "some": true,
	"them": true, "then": true, "these": true, "thing": true, "where": true,
	"much": true, "should": true, "well": true, "after": true,
	// Portuguese stop words
	"que": true, "não": true, "com": true, "uma": true, "para": true,
	"por": true, "mais": true, "como": true, "mas": true, "dos": true,
	"das": true, "nos": true, "nas": true, "foi": true, "ser": true,
	"tem": true, "são": true, "seu": true, "sua": true, "isso": true,
	"este": true, "esta": true, "esse": true, "essa": true, "aqui": true,
	"ele": true, "ela": true, "eles": true, "elas": true, "nós": true,
	"vocé": true, "voce": true, "você": true, "também": true,
}

// searchLikeFallback performs a simple LIKE search when FTS5 is not available.
func (s *SQLiteStore) searchLikeFallback(query string, maxResults int) ([]SearchResult, error) {
	// Split query into words and search for each with LIKE.
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, nil
	}

	// Build a query that matches any word.
	var conditions []string
	var args []any
	for _, w := range words {
		conditions = append(conditions, "LOWER(text) LIKE ?")
		args = append(args, "%"+w+"%")
	}
	args = append(args, maxResults*2)

	sqlQuery := fmt.Sprintf(`
		SELECT file_id, text FROM chunks
		WHERE %s
		LIMIT ?
	`, strings.Join(conditions, " OR "))

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("LIKE search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.FileID, &r.Text); err != nil {
			continue
		}
		// Score based on word match count.
		text := strings.ToLower(r.Text)
		matches := 0
		for _, w := range words {
			if strings.Contains(text, w) {
				matches++
			}
		}
		r.Score = float64(matches) / float64(len(words))
		results = append(results, r)
	}

	return results, nil
}

// SearchVector performs a vector similarity search using in-memory cosine similarity.
func (s *SQLiteStore) SearchVector(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if s.embedder.Name() == "none" {
		return nil, nil
	}

	// Generate query embedding.
	embeddings, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, nil
	}
	queryVec := embeddings[0]

	// Search in-memory cache.
	s.vectorCacheMu.RLock()
	cache := s.vectorCache
	s.vectorCacheMu.RUnlock()

	if len(cache) == 0 {
		return nil, nil
	}

	type scored struct {
		entry vectorCacheEntry
		score float64
	}
	var candidates []scored

	for _, entry := range cache {
		if len(entry.embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, entry.embedding)
		if sim > 0 {
			candidates = append(candidates, scored{entry: entry, score: sim})
		}
	}

	// Sort by score descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if maxResults <= 0 {
		maxResults = 10
	}
	if len(candidates) > maxResults*2 {
		candidates = candidates[:maxResults*2]
	}

	var results []SearchResult
	for _, c := range candidates {
		results = append(results, SearchResult{
			FileID: c.entry.fileID,
			Text:   c.entry.text,
			Score:  c.score,
		})
	}

	return results, nil
}

// HybridSearch performs a combined vector + BM25 search with configurable weights.
func (s *SQLiteStore) HybridSearch(ctx context.Context, query string, maxResults int, minScore float64, vectorWeight, bm25Weight float64) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 6
	}
	if minScore <= 0 {
		minScore = 0.1
	}
	if vectorWeight <= 0 {
		vectorWeight = 0.7
	}
	if bm25Weight <= 0 {
		bm25Weight = 0.3
	}

	// Run both searches in parallel.
	type searchResult struct {
		results []SearchResult
		err     error
	}

	vecCh := make(chan searchResult, 1)
	bm25Ch := make(chan searchResult, 1)

	go func() {
		results, err := s.SearchVector(ctx, query, maxResults*4)
		vecCh <- searchResult{results, err}
	}()

	go func() {
		results, err := s.SearchBM25(query, maxResults*4)
		bm25Ch <- searchResult{results, err}
	}()

	vecResult := <-vecCh
	bm25Result := <-bm25Ch

	// Merge results using Reciprocal Rank Fusion (RRF).
	// Use a hash of the full text as merge key to avoid collisions between
	// different chunks from the same file that share a common prefix.
	scoreMap := make(map[string]*SearchResult) // key = sha256(fileID + text)

	mergeResults := func(results []SearchResult, weight float64) {
		for i, r := range results {
			key := hashText(r.FileID + "|" + r.Text)
			if existing, ok := scoreMap[key]; ok {
				existing.Score += weight * (1.0 / float64(i+1))
			} else {
				scoreMap[key] = &SearchResult{
					FileID: r.FileID,
					Text:   r.Text,
					Score:  weight * (1.0 / float64(i+1)),
				}
			}
		}
	}

	if vecResult.err == nil {
		mergeResults(vecResult.results, vectorWeight)
	}
	if bm25Result.err == nil {
		mergeResults(bm25Result.results, bm25Weight)
	}

	// Collect and sort by combined score.
	var merged []SearchResult
	for _, r := range scoreMap {
		if r.Score >= minScore {
			merged = append(merged, *r)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if len(merged) > maxResults {
		merged = merged[:maxResults]
	}

	return merged, nil
}

// TemporalDecayConfig configures exponential score decay based on memory age.
type TemporalDecayConfig struct {
	Enabled      bool
	HalfLifeDays float64
}

// ApplyTemporalDecay applies exponential decay to search results based on file age.
// Files matching the pattern "memory/YYYY-MM-DD.md" are decayed; evergreen files
// (MEMORY.md or non-dated) are not decayed.
func (s *SQLiteStore) ApplyTemporalDecay(results []SearchResult, cfg TemporalDecayConfig) []SearchResult {
	if !cfg.Enabled || len(results) == 0 {
		return results
	}

	halfLife := cfg.HalfLifeDays
	if halfLife <= 0 {
		halfLife = 30
	}
	lambda := math.Log(2) / halfLife
	now := time.Now()

	for i := range results {
		fileDate := extractDateFromFileID(results[i].FileID)
		if fileDate == nil {
			continue // Evergreen file, no decay
		}

		ageDays := now.Sub(*fileDate).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
		decayFactor := math.Exp(-lambda * ageDays)
		results[i].Score *= decayFactor
	}

	return results
}

// extractDateFromFileID extracts a date from file IDs like "memory/2026-02-25.md"
// or "2026-02-25.md". Returns nil for evergreen files (MEMORY.md or non-dated).
func extractDateFromFileID(fileID string) *time.Time {
	// Evergreen files don't decay
	if strings.Contains(fileID, "MEMORY.md") {
		return nil
	}

	// Extract base filename
	base := fileID
	if idx := strings.LastIndex(fileID, "/"); idx >= 0 {
		base = fileID[idx+1:]
	}

	// Remove extension
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}

	// Try to parse as date (YYYY-MM-DD)
	t, err := time.Parse("2006-01-02", base)
	if err != nil {
		return nil
	}
	return &t
}

// MMRConfig configures Maximal Marginal Relevance for search diversification.
type MMRConfig struct {
	Enabled bool
	Lambda  float64
}

// ApplyMMR applies Maximal Marginal Relevance re-ranking to diversify results.
// Lambda controls the balance: 0 = max diversity, 1 = max relevance.
func (s *SQLiteStore) ApplyMMR(results []SearchResult, cfg MMRConfig, maxResults int) []SearchResult {
	if !cfg.Enabled || len(results) <= maxResults {
		return results
	}

	lambda := cfg.Lambda
	if lambda <= 0 {
		lambda = 0.7
	}
	if lambda > 1 {
		lambda = 1
	}

	selected := make([]SearchResult, 0, maxResults)
	remaining := make([]SearchResult, len(results))
	copy(remaining, results)

	// First: highest relevance
	selected = append(selected, remaining[0])
	remaining = remaining[1:]

	// Pre-tokenize for Jaccard similarity
	tokenCache := make(map[string]map[string]bool)
	tokenize := func(text string) map[string]bool {
		if cached, ok := tokenCache[text]; ok {
			return cached
		}
		tokens := make(map[string]bool)
		for _, word := range strings.Fields(strings.ToLower(text)) {
			if len(word) > 2 {
				tokens[word] = true
			}
		}
		tokenCache[text] = tokens
		return tokens
	}

	for len(selected) < maxResults && len(remaining) > 0 {
		bestIdx := 0
		bestScore := -1.0

		for i, candidate := range remaining {
			// MMR = lambda * relevance - (1-lambda) * max_similarity_to_selected
			maxSim := 0.0
			candidateTokens := tokenize(candidate.Text)
			for _, sel := range selected {
				sim := jaccardSimilarity(candidateTokens, tokenize(sel.Text))
				if sim > maxSim {
					maxSim = sim
				}
			}

			mmrScore := lambda*candidate.Score - (1-lambda)*maxSim
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

// jaccardSimilarity computes Jaccard similarity between two token sets.
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	for token := range a {
		if b[token] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// HybridSearchWithOptions performs hybrid search with optional temporal decay and MMR.
func (s *SQLiteStore) HybridSearchWithOptions(ctx context.Context, query string, maxResults int, minScore float64, vectorWeight, bm25Weight float64, decayCfg TemporalDecayConfig, mmrCfg MMRConfig) ([]SearchResult, error) {
	results, err := s.HybridSearch(ctx, query, maxResults*2, minScore, vectorWeight, bm25Weight)
	if err != nil {
		return nil, err
	}

	// Apply temporal decay
	results = s.ApplyTemporalDecay(results, decayCfg)

	// Re-sort after decay
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply MMR for diversification
	results = s.ApplyMMR(results, mmrCfg, maxResults)

	return results, nil
}

// getEmbeddingCache looks up a cached embedding by text hash.
func (s *SQLiteStore) getEmbeddingCache(text string) []float32 {
	hash := hashText(text)
	var embJSON string
	err := s.db.QueryRow(`
		SELECT embedding FROM embedding_cache
		WHERE text_hash = ? AND provider = ? AND model = ?
	`, hash, s.embedder.Name(), s.embedder.Model()).Scan(&embJSON)
	if err != nil {
		return nil
	}
	var emb []float32
	if err := json.Unmarshal([]byte(embJSON), &emb); err != nil {
		return nil
	}
	return emb
}

// setEmbeddingCache stores an embedding in the cache.
func (s *SQLiteStore) setEmbeddingCache(text string, embedding []float32) {
	hash := hashText(text)
	data, err := json.Marshal(embedding)
	if err != nil {
		return
	}
	_, _ = s.db.Exec(`
		INSERT INTO embedding_cache (text_hash, provider, model, embedding, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(text_hash, provider, model) DO UPDATE SET
			embedding = excluded.embedding, updated_at = CURRENT_TIMESTAMP
	`, hash, s.embedder.Name(), s.embedder.Model(), string(data))
}

// refreshVectorCache loads all chunk embeddings into memory for fast search.
func (s *SQLiteStore) refreshVectorCache() error {
	rows, err := s.db.Query("SELECT id, file_id, text, embedding FROM chunks WHERE embedding IS NOT NULL")
	if err != nil {
		return err
	}
	defer rows.Close()

	var cache []vectorCacheEntry
	for rows.Next() {
		var e vectorCacheEntry
		var embJSON string
		if err := rows.Scan(&e.chunkID, &e.fileID, &e.text, &embJSON); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(embJSON), &e.embedding); err != nil {
			continue
		}
		cache = append(cache, e)
	}

	s.vectorCacheMu.Lock()
	s.vectorCache = cache
	s.vectorCacheMu.Unlock()

	s.logger.Debug("vector cache refreshed", "chunks", len(cache))
	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ChunkCount returns the total number of indexed chunks.
func (s *SQLiteStore) ChunkCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&count)
	return count
}

// FileCount returns the total number of indexed files.
func (s *SQLiteStore) FileCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	return count
}

// PruneEmbeddingCache removes old cache entries exceeding maxEntries.
func (s *SQLiteStore) PruneEmbeddingCache(maxEntries int) {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	_, _ = s.db.Exec(`
		DELETE FROM embedding_cache WHERE rowid IN (
			SELECT rowid FROM embedding_cache
			ORDER BY updated_at DESC
			LIMIT -1 OFFSET ?
		)
	`, maxEntries)
}

// ---------- Math Helpers ----------

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// hashText computes the SHA-256 hex hash of a text for cache keying.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// truncateText returns the first n characters of text.
func truncateText(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[:n]
}

// sanitizeFTS5Query escapes FTS5 special characters and wraps the query in
// double quotes so it is treated as a phrase literal. This prevents accidental
// FTS5 syntax injection from user input.
func sanitizeFTS5Query(query string) string {
	// Remove characters that are FTS5 operators.
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '"', '(', ')', '*', '^', ':', '{', '}':
			return ' '
		default:
			return r
		}
	}, query)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	// Wrap in double quotes for phrase matching.
	return `"` + cleaned + `"`
}

// IndexMemoryDir indexes all .md files in the memory directory and MEMORY.md.
func (s *SQLiteStore) IndexMemoryDir(ctx context.Context, memDir string, chunkCfg ChunkConfig) error {
	start := time.Now()

	fileChunks, err := IndexDirectory(memDir, chunkCfg)
	if err != nil {
		return fmt.Errorf("index directory: %w", err)
	}

	for fileID, chunks := range fileChunks {
		fHash := ""
		for _, c := range chunks {
			fHash += c.Hash
		}
		if err := s.IndexChunks(ctx, fileID, chunks, fHash); err != nil {
			s.logger.Warn("failed to index file", "file", fileID, "error", err)
		}
	}

	s.logger.Info("memory index complete",
		"files", len(fileChunks),
		"chunks", s.ChunkCount(),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return nil
}
