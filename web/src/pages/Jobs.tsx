import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Clock, Play, Pause, AlertCircle, Timer, Wrench, CalendarClock, FileCode } from 'lucide-react'
import { api, type JobInfo } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

export function Jobs() {
  const { t } = useTranslation()
  const [jobs, setJobs] = useState<JobInfo[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.jobs.list()
      .then(setJobs)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center bg-dc-darker">
        <div className="h-8 w-8 rounded-full border-4 border-blue-500/30 border-t-blue-500 animate-spin" />
      </div>
    )
  }

  const active = jobs.filter((j) => j.enabled)
  const inactive = jobs.filter((j) => !j.enabled)

  return (
    <div className="flex-1 overflow-y-auto bg-dc-darker">
      <div className="mx-auto max-w-3xl px-8 py-10">
        {/* Header */}
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-zinc-600">{t('jobs.subtitle')}</p>
          <h1 className="mt-1 text-2xl font-black text-white tracking-tight">{t('jobs.title')}</h1>
        </div>

        {jobs.length === 0 ? (
          <EmptyJobs />
        ) : (
          <div className="mt-6 space-y-6">
            {active.length > 0 && (
              <div className="space-y-2">
                {active.map((job) => <JobCard key={job.id} job={job} />)}
              </div>
            )}
            {inactive.length > 0 && (
              <div>
                {active.length > 0 && (
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-600">{t('common.disabled')}</p>
                )}
                <div className="space-y-2">
                  {inactive.map((job) => <JobCard key={job.id} job={job} />)}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function EmptyJobs() {
  const { t } = useTranslation()
  return (
    <div className="mt-8 rounded-2xl border border-white/6 bg-(--color-dc-dark)/80 px-6 py-12">
      <div className="flex flex-col items-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-blue-500/10">
          <CalendarClock className="h-6 w-6 text-blue-400" />
        </div>
        <h3 className="mt-4 text-sm font-bold text-zinc-300">{t('jobs.noJobs')}</h3>
        <p className="mt-1.5 max-w-sm text-center text-xs text-zinc-500">
          {t('jobs.subtitle')}
        </p>
      </div>

      <div className="mt-6 mx-auto max-w-md rounded-xl bg-zinc-800/30 px-4 py-3 ring-1 ring-zinc-700/20">
        <p className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">config.yaml</p>
        <pre className="mt-2 overflow-x-auto font-mono text-xs leading-relaxed text-zinc-400">
{`scheduler:
  jobs:
    - id: backup-diario
      schedule: "0 2 * * *"
      type: prompt
      command: "Faça backup do banco"
      enabled: true`}</pre>
      </div>

      <div className="mt-4 flex items-center justify-center gap-4 text-[11px] text-zinc-600">
        <span className="flex items-center gap-1.5">
          <Clock className="h-3 w-3 text-zinc-500" />
          Cron syntax padrão
        </span>
        <span className="h-3 w-px bg-zinc-700/50" />
        <span className="flex items-center gap-1.5">
          <FileCode className="h-3 w-3 text-zinc-500" />
          Tipos: prompt, command, skill
        </span>
      </div>
    </div>
  )
}

function JobCard({ job }: { job: JobInfo }) {
  const hasRun = job.last_run_at && job.last_run_at !== '0001-01-01T00:00:00Z'

  return (
    <div className={`rounded-xl px-5 py-4 ring-1 transition-colors ${
      job.enabled
        ? 'bg-emerald-500/3 ring-emerald-500/15'
        : 'bg-zinc-800/30 ring-zinc-700/20'
    }`}>
      <div className="flex items-start gap-4">
        <div className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${
          job.enabled ? 'bg-emerald-500/10' : 'bg-zinc-800'
        }`}>
          {job.enabled ? <Play className="h-4 w-4 text-emerald-400" /> : <Pause className="h-4 w-4 text-zinc-500" />}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h3 className="text-sm font-bold text-white">{job.id}</h3>
            <span className="rounded bg-zinc-800 px-1.5 py-0.5 font-mono text-[10px] text-blue-400 ring-1 ring-zinc-700/30">
              {job.schedule}
            </span>
            <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 ring-1 ring-zinc-700/30">
              {job.type}
            </span>
          </div>

          {job.command && (
            <pre className="mt-2 overflow-x-auto rounded-lg bg-zinc-900/60 px-3 py-2 font-mono text-xs text-zinc-400 ring-1 ring-zinc-700/15">
              {job.command}
            </pre>
          )}

          <div className="mt-2.5 flex flex-wrap items-center gap-3 text-[11px] text-zinc-500">
            <span className="flex items-center gap-1">
              <Wrench className="h-3 w-3" />
              {job.run_count} execuções
            </span>
            {hasRun && (
              <span className="flex items-center gap-1">
                <Timer className="h-3 w-3" />
                Última: {timeAgo(job.last_run_at)}
              </span>
            )}
            {job.last_error && (
              <span className="flex items-center gap-1 text-amber-400">
                <AlertCircle className="h-3 w-3" />
                {job.last_error}
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
