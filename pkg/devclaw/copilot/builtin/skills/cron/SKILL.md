---
name: cron
description: "Schedule jobs to run at specific times or intervals"
trigger: automatic
---

# Cron / Agendamentos

Schedule **jobs** (agendamentos) to execute commands at specific times or intervals.

## Terminologia

| Termo | Significado |
|-------|-------------|
| **Job** | Um agendamento (tarefa agendada) |
| **Tools** | `cron_add`, `cron_list`, `cron_remove` |
| **Types** | `at` (única vez), `every` (intervalo), `cron` (expressão cron) |

## Tools

| Tool | Action |
|------|--------|
| `cron_add` | Criar um novo job agendado |
| `cron_list` | Listar todos os jobs |
| `cron_remove` | Remover um job pelo ID |

## Tipos de Agendamento

### Type: `at` (Única Vez)
Executa uma única vez e remove automaticamente. Use para lembretes e tarefas adiadas.

```bash
cron_add(
  id="lembrete-reuniao",
  type="at",
  schedule="30m",           # Em 30 minutos
  command="Lembrar: reunião em 5 minutos"
)

cron_add(
  id="callback-cliente",
  type="at",
  schedule="14:30",         # Hoje às 14:30
  command="Ligar para cliente sobre proposta"
)

cron_add(
  id="review-sexta",
  type="at",
  schedule="2026-02-28 09:00",  # Data específica
  command="Revisar relatório semanal"
)
```

### Type: `every` (Intervalo Recorrente)
Executa repetidamente em intervalos fixos.

```bash
cron_add(
  id="health-check",
  type="every",
  schedule="5m",            # A cada 5 minutos
  command="Verificar status do servidor"
)

cron_add(
  id="sync-data",
  type="every",
  schedule="1h",            # A cada hora
  command="Sincronizar dados com API externa"
)
```

### Type: `cron` (Expressão Cron)
Executa baseado em expressão cron padrão.

```bash
cron_add(
  id="daily-report",
  type="cron",
  schedule="0 9 * * *",     # Todo dia às 9:00
  command="Gerar relatório diário"
)

cron_add(
  id="weekly-backup",
  type="cron",
  schedule="0 2 * * 0",     # Domingo às 2:00
  command="Executar backup semanal"
)

cron_add(
  id="monthly-invoice",
  type="cron",
  schedule="0 9 1 * *",     # Dia 1 de cada mês às 9:00
  command="Enviar faturas mensais"
)
```

## Expressões Cron

```
┌───────────── minuto (0-59)
│ ┌───────────── hora (0-23)
│ │ ┌───────────── dia do mês (1-31)
│ │ │ ┌───────────── mês (1-12)
│ │ │ │ ┌───────────── dia da semana (0-6, 0=domingo)
│ │ │ │ │
* * * * *
```

| Expressão | Significado |
|-----------|-------------|
| `*/5 * * * *` | A cada 5 minutos |
| `0 * * * *` | Toda hora |
| `0 9 * * *` | Todo dia às 9:00 |
| `0 9 * * 1-5` | Dias úteis às 9:00 |
| `0 9,17 * * *` | Duas vezes ao dia (9h e 17h) |
| `0 2 * * 0` | Domingos às 2:00 |
| `0 9 1 * *` | Dia 1 de cada mês |

## Listar Jobs

```bash
cron_list()
# Output:
# Scheduled jobs (3):
# 1. [at] lembrete-reuniao: 30m → "Lembrar: reunião..."
# 2. [every] health-check: 5m → "Verificar status..."
# 3. [cron] daily-report: 0 9 * * * → "Gerar relatório..."
```

## Remover Job

```bash
cron_remove(id="health-check")
# Output: Job 'health-check' removed
```

## Parâmetros do `cron_add`

| Parâmetro | Obrigatório | Descrição |
|-----------|-------------|-----------|
| `id` | Sim | Identificador único do job |
| `type` | Não | `at`, `every`, ou `cron` (padrão: `cron`) |
| `schedule` | Sim | Horário/intervalo (formato depende do type) |
| `command` | Sim | Comando/prompt a executar |
| `channel` | Não | Canal de resposta (ex: `whatsapp`) |
| `chat_id` | Não | ID do chat/grupo para resposta |

## Formatos de Schedule por Type

| Type | Formato | Exemplos |
|------|---------|----------|
| `at` | Duração relativa ou horário absoluto | `5m`, `1h`, `14:30`, `2026-03-01 09:00` |
| `every` | Intervalo | `5m`, `30m`, `1h`, `1d` |
| `cron` | Expressão cron | `0 9 * * *`, `*/10 * * * *` |

## Workflow Completo

### Lembrete Simples
```bash
# Usuário: "Me lembra de ligar pro cliente em 20 minutos"
cron_add(
  id="lembrete-ligacao",
  type="at",
  schedule="20m",
  command="Ligar para o cliente sobre a proposta"
)
```

### Verificação Periódica
```bash
# Verificar sistema a cada 10 minutos
cron_add(
  id="monitor-api",
  type="every",
  schedule="10m",
  command="Verificar se a API está respondendo e reportar se houver erro"
)
```

### Relatório Diário
```bash
# Todo dia às 8:00
cron_add(
  id="morning-briefing",
  type="cron",
  schedule="0 8 * * *",
  command="Gerar resumo das tarefas do dia e enviar"
)
```

## Importante

| Nota | Motivo |
|------|--------|
| Jobs persistem após restart | Salvos em configuração |
| `at` remove automaticamente | Executa uma vez e some |
| Use IDs descritivos | Facilita gerenciar depois |
| `channel`/`chat_id` são opcionais | Usam contexto atual por padrão |

## Erros Comuns

| Erro | Correção |
|------|----------|
| Usar `cron` para lembrete único | Use `type="at"` |
| Esquecer de especificar `type` | Padrão é `cron`, pode não ser o desejado |
| Schedule inválido para o type | Cada type aceita formato diferente |
| ID duplicado | Sobrescreve job existente |
