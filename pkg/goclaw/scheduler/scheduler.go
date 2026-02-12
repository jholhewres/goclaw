// Package scheduler implementa o sistema de agendamento de tarefas do AgentGo Copilot.
// Utiliza expressões cron para agendar execução de prompts/comandos e envio dos
// resultados para canais específicos.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Scheduler gerencia tarefas agendadas utilizando expressões cron.
type Scheduler struct {
	// jobs armazena os jobs registrados, indexados por ID.
	jobs map[string]*Job

	// storage persiste jobs em disco/banco.
	storage JobStorage

	// handler é chamado quando um job deve ser executado.
	handler JobHandler

	logger *slog.Logger
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// Job representa uma tarefa agendada.
type Job struct {
	// ID é o identificador único do job.
	ID string `json:"id" yaml:"id"`

	// Schedule é a expressão cron que define quando o job executa.
	// Suporta formato cron padrão (5 campos) e extensões como "@daily", "@hourly".
	Schedule string `json:"schedule" yaml:"schedule"`

	// Command é o prompt/comando a ser executado pelo agente.
	Command string `json:"command" yaml:"command"`

	// Channel é o canal onde o resultado será enviado (ex: "whatsapp").
	Channel string `json:"channel" yaml:"channel"`

	// ChatID é o identificador do chat/grupo destino.
	ChatID string `json:"chat_id" yaml:"chat_id"`

	// Enabled indica se o job está ativo.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CreatedAt é o timestamp de criação.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// LastRunAt é o timestamp da última execução.
	LastRunAt *time.Time `json:"last_run_at,omitempty" yaml:"last_run_at,omitempty"`

	// LastError contém o erro da última execução, se houve.
	LastError string `json:"last_error,omitempty" yaml:"last_error,omitempty"`
}

// JobHandler é a função chamada quando um job deve ser executado.
// Recebe o job e retorna o resultado (resposta do agente) ou erro.
type JobHandler func(ctx context.Context, job *Job) (string, error)

// JobStorage define a interface para persistência de jobs.
type JobStorage interface {
	// Save persiste um job.
	Save(job *Job) error

	// Delete remove um job pelo ID.
	Delete(id string) error

	// LoadAll carrega todos os jobs persistidos.
	LoadAll() ([]*Job, error)
}

// New cria um novo Scheduler com o storage e handler fornecidos.
func New(storage JobStorage, handler JobHandler, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		jobs:    make(map[string]*Job),
		storage: storage,
		handler: handler,
		logger:  logger,
	}
}

// Add adiciona um novo job ao scheduler.
func (s *Scheduler) Add(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.ID == "" {
		return fmt.Errorf("job ID é obrigatório")
	}

	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("job %q já existe", job.ID)
	}

	job.CreatedAt = time.Now()
	s.jobs[job.ID] = job

	if s.storage != nil {
		if err := s.storage.Save(job); err != nil {
			s.logger.Error("falha ao persistir job", "id", job.ID, "error", err)
		}
	}

	s.logger.Info("job adicionado",
		"id", job.ID,
		"schedule", job.Schedule,
		"channel", job.Channel,
	)
	return nil
}

// Remove remove um job pelo ID.
func (s *Scheduler) Remove(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[jobID]; !exists {
		return fmt.Errorf("job %q não encontrado", jobID)
	}

	delete(s.jobs, jobID)

	if s.storage != nil {
		if err := s.storage.Delete(jobID); err != nil {
			s.logger.Error("falha ao remover job do storage", "id", jobID, "error", err)
		}
	}

	s.logger.Info("job removido", "id", jobID)
	return nil
}

// List retorna todos os jobs registrados.
func (s *Scheduler) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

// Start inicia o scheduler e carrega jobs persistidos.
func (s *Scheduler) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Carrega jobs persistidos sob lock para evitar race com Add/Remove.
	if s.storage != nil {
		jobs, err := s.storage.LoadAll()
		if err != nil {
			s.logger.Error("falha ao carregar jobs", "error", err)
		} else {
			s.mu.Lock()
			for _, job := range jobs {
				s.jobs[job.ID] = job
			}
			s.mu.Unlock()
			s.logger.Info("jobs carregados do storage", "count", len(jobs))
		}
	}

	s.mu.RLock()
	jobCount := len(s.jobs)
	s.mu.RUnlock()

	// TODO: Integrar com robfig/cron para execução real dos jobs.
	// Por enquanto, o loop de execução será implementado na próxima fase.
	s.logger.Info("scheduler iniciado", "jobs", jobCount)
	return nil
}

// Stop para o scheduler de forma graciosa.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.logger.Info("scheduler encerrado")
}
