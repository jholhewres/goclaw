// Package copilot – vault.go provides encrypted credential storage using
// AES-256-GCM with Argon2id key derivation. Secrets are stored in a local
// file (.devclaw.vault) that is unreadable without the master password.
//
// Even if someone has filesystem access, the vault contents remain encrypted.
// The master password is never stored — only a derived key is used in memory.
package copilot

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

const (
	// VaultFile is the default vault file name.
	VaultFile = ".devclaw.vault"

	// Argon2id parameters (OWASP recommended).
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32 // AES-256

	// saltLen is the length of the random salt for Argon2.
	saltLen = 16
)

// VaultEntry holds one encrypted secret.
type VaultEntry struct {
	Nonce      string `json:"nonce"`      // base64-encoded AES-GCM nonce
	Ciphertext string `json:"ciphertext"` // base64-encoded encrypted data
}

// VaultData is the on-disk format of the vault.
type VaultData struct {
	Version int                   `json:"version"`
	Salt    string                `json:"salt"` // base64-encoded Argon2 salt
	Entries map[string]VaultEntry `json:"entries"`
}

// Vault provides encrypted secret storage backed by a local file.
type Vault struct {
	path      string
	data      *VaultData
	derivedKey []byte // 32-byte AES key (only in memory while unlocked)
	mu        sync.RWMutex
}

// NewVault creates a vault instance pointing to the given file path.
// The vault is not yet unlocked — call Unlock() or Create() first.
func NewVault(path string) *Vault {
	return &Vault{path: path}
}

// Exists returns true if the vault file exists on disk.
func (v *Vault) Exists() bool {
	_, err := os.Stat(v.path)
	return err == nil
}

// Path returns the vault file path.
func (v *Vault) Path() string {
	return v.path
}

// IsUnlocked returns true if the vault has been unlocked with a password.
func (v *Vault) IsUnlocked() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.derivedKey != nil
}

// Create initializes a new vault with the given master password.
// If the vault file already exists, it returns an error.
func (v *Vault) Create(password string) error {
	if v.Exists() {
		return fmt.Errorf("vault already exists at %s", v.path)
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.derivedKey = deriveKey(password, salt)
	v.data = &VaultData{
		Version: 1,
		Salt:    base64.StdEncoding.EncodeToString(salt),
		Entries: make(map[string]VaultEntry),
	}

	return v.saveLocked()
}

// Unlock decrypts and loads the vault using the provided master password.
// Returns an error if the password is wrong (decryption will fail on
// the verification entry or the first real entry).
func (v *Vault) Unlock(password string) error {
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return fmt.Errorf("reading vault: %w", err)
	}

	var data VaultData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing vault: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(data.Salt)
	if err != nil {
		return fmt.Errorf("decoding salt: %w", err)
	}

	key := deriveKey(password, salt)

	// Verify the password by trying to decrypt the verification entry.
	if verify, ok := data.Entries["__verify__"]; ok {
		if _, err := decryptEntry(key, verify); err != nil {
			return fmt.Errorf("wrong password")
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.derivedKey = key
	v.data = &data

	return nil
}

// Lock clears the derived key from memory, locking the vault.
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Zero out the key before discarding.
	if v.derivedKey != nil {
		for i := range v.derivedKey {
			v.derivedKey[i] = 0
		}
	}
	v.derivedKey = nil
}

// Set stores a secret in the vault (encrypted). The vault must be unlocked.
func (v *Vault) Set(name, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.derivedKey == nil {
		return fmt.Errorf("vault is locked")
	}

	entry, err := encryptEntry(v.derivedKey, []byte(value))
	if err != nil {
		return fmt.Errorf("encrypting %s: %w", name, err)
	}

	v.data.Entries[name] = entry

	// Ensure we have a verification entry.
	if _, ok := v.data.Entries["__verify__"]; !ok {
		ve, _ := encryptEntry(v.derivedKey, []byte("devclaw-vault-ok"))
		v.data.Entries["__verify__"] = ve
	}

	return v.saveLocked()
}

// Get retrieves and decrypts a secret from the vault. Returns empty string
// if the key doesn't exist. The vault must be unlocked.
func (v *Vault) Get(name string) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.derivedKey == nil {
		return "", fmt.Errorf("vault is locked")
	}

	entry, ok := v.data.Entries[name]
	if !ok {
		return "", nil
	}

	plaintext, err := decryptEntry(v.derivedKey, entry)
	if err != nil {
		return "", fmt.Errorf("decrypting %s: %w", name, err)
	}

	return string(plaintext), nil
}

// Has returns true if a secret exists in the vault.
// Returns false if vault is locked or key doesn't exist.
func (v *Vault) Has(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.derivedKey == nil || v.data == nil {
		return false
	}

	_, ok := v.data.Entries[name]
	return ok
}

// Delete removes a secret from the vault. The vault must be unlocked.
func (v *Vault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.derivedKey == nil {
		return fmt.Errorf("vault is locked")
	}

	delete(v.data.Entries, name)
	return v.saveLocked()
}

// Keys returns the names of all stored secrets (excluding internal entries).
func (v *Vault) Keys() ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.derivedKey == nil {
		return nil, fmt.Errorf("vault is locked")
	}

	var keys []string
	for k := range v.data.Entries {
		if k == "__verify__" {
			continue
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// List returns the names of all stored secrets (excluding internal entries).
// Returns an empty slice if the vault is locked or empty.
func (v *Vault) List() []string {
	keys, err := v.Keys()
	if err != nil {
		return nil
	}
	return keys
}

// InjectProviderKeys injects all secrets from the vault into environment variables.
// This allows LLM clients to find their API keys via standard variable names
// (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.) without the config needing to
// reference them explicitly.
//
// The vault must be unlocked before calling this method.
func (v *Vault) InjectProviderKeys() error {
	if !v.IsUnlocked() {
		return fmt.Errorf("vault is locked")
	}

	keys := v.List()
	for _, key := range keys {
		val, err := v.Get(key)
		if err != nil || val == "" {
			continue
		}

		// Inject with uppercase key name as environment variable.
		// e.g., "OPENAI_API_KEY" -> OPENAI_API_KEY env var
		os.Setenv(key, val)
	}

	return nil
}

// ChangePassword re-encrypts all entries with a new master password.
// The vault must be unlocked.
func (v *Vault) ChangePassword(newPassword string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.derivedKey == nil {
		return fmt.Errorf("vault is locked")
	}

	// Decrypt all entries with the old key.
	decrypted := make(map[string][]byte)
	for name, entry := range v.data.Entries {
		plaintext, err := decryptEntry(v.derivedKey, entry)
		if err != nil {
			return fmt.Errorf("decrypting %s: %w", name, err)
		}
		decrypted[name] = plaintext
	}

	// Generate new salt and derive new key.
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}

	newKey := deriveKey(newPassword, salt)

	// Re-encrypt all entries with the new key.
	newEntries := make(map[string]VaultEntry, len(decrypted))
	for name, plaintext := range decrypted {
		entry, err := encryptEntry(newKey, plaintext)
		if err != nil {
			return fmt.Errorf("re-encrypting %s: %w", name, err)
		}
		newEntries[name] = entry
	}

	// Zero old key.
	for i := range v.derivedKey {
		v.derivedKey[i] = 0
	}

	v.derivedKey = newKey
	v.data.Salt = base64.StdEncoding.EncodeToString(salt)
	v.data.Entries = newEntries

	return v.saveLocked()
}

// ---------- Internal ----------

// deriveKey uses Argon2id to derive a 32-byte AES key from a password and salt.
func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		argonKeyLen,
	)
}

// encryptEntry encrypts plaintext using AES-256-GCM with a random nonce.
func encryptEntry(key, plaintext []byte) (VaultEntry, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return VaultEntry{}, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return VaultEntry{}, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return VaultEntry{}, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return VaultEntry{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

// decryptEntry decrypts a VaultEntry using AES-256-GCM.
func decryptEntry(key []byte, entry VaultEntry) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(entry.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decoding nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(entry.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?)")
	}

	return plaintext, nil
}

// saveLocked writes the vault to disk. Caller must hold v.mu.
func (v *Vault) saveLocked() error {
	data, err := json.MarshalIndent(v.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling vault: %w", err)
	}

	// Write with restricted permissions (owner read/write only).
	if err := os.WriteFile(v.path, data, 0o600); err != nil {
		return fmt.Errorf("writing vault: %w", err)
	}

	return nil
}

// ReadPassword reads a password from the terminal without echoing.
// Falls back to regular stdin reading if terminal is not available.
func ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	// Try terminal-based password reading (no echo).
	fd := int(os.Stdin.Fd())
	password, err := term.ReadPassword(fd)
	if err != nil {
		// Fallback: read from stdin (with echo — for piped input or non-TTY).
		var buf [1024]byte
		n, readErr := os.Stdin.Read(buf[:])
		if readErr != nil {
			return "", fmt.Errorf("reading password: %w", readErr)
		}
		password = buf[:n]
	}

	fmt.Println() // Move to next line after hidden input.

	// Trim newline.
	s := string(password)
	s = trimNewlines(s)
	return s, nil
}

// trimNewlines removes trailing \n and \r from a string.
func trimNewlines(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
