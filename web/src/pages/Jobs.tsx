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
      <div className="flex flex-1 items-center justify-center bg-[#0c1222]">
        <div className="h-8 w-8 rounded-full border-4 border-[#1e293b] border-t-[#3b82f6] animate-spin" />
      </div>
    )
  }

  const active = jobs.filter((j) => j.enabled)
  const inactive = jobs.filter((j) => !j.enabled)

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <div>
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{t('jobs.subtitle')}</p>
        <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">{t('jobs.title')}</h1>
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
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-[#475569]">{t('common.disabled')}</p>
                )}
                <div className="space-y-2">
                  {inactive.map((job) => <JobCard key={job.id} job={job} />)}
                </div>
              </div>
            )}
          </div>
        )}
    </div>
  )
}

function EmptyJobs() {
  const { t } = useTranslation()
  return (
    <div className="mt-8 rounded-2xl border border-white/10 bg-[#111827] px-6 py-12">
      <div className="flex flex-col items-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-[#1e293b]">
          <CalendarClock className="h-6 w-6 text-[#64748b]" />
        </div>
        <h3 className="mt-4 text-sm font-semibold text-[#94a3b8]">{t('jobs.noJobs')}</h3>
        <p className="mt-1.5 max-w-sm text-center text-xs text-[#64748b]">
          {t('jobs.subtitle')}
        </p>
      </div>

      <div className="mt-6 mx-auto max-w-md rounded-xl bg-[#0c1222] px-4 py-3 border border-white/5">
        <p className="text-[11px] font-semibold uppercase tracking-wider text-[#475569]">config.yaml</p>
        <pre className="mt-2 overflow-x-auto font-mono text-xs leading-relaxed text-[#94a3b8]">
{`scheduler:
  jobs:
    - id: backup-diario
      schedule: "0 2 * * *"
      type: prompt
      command: "Faça backup do banco"
      enabled: true`}</pre>
      </div>

      <div className="mt-4 flex items-center justify-center gap-4 text-[11px] text-[#475569]">
        <span className="flex items-center gap-1.5">
          <Clock className="h-3 w-3 text-[#64748b]" />
          Cron syntax padrão
        </span>
        <span className="h-3 w-px bg-white/10" />
        <span className="flex items-center gap-1.5">
          <FileCode className="h-3 w-3 text-[#64748b]" />
          Tipos: prompt, command, skill
        </span>
      </div>
    </div>
  )
}

function JobCard({ job }: { job: JobInfo }) {
  const hasRun = job.last_run_at && job.last_run_at !== '0001-01-01T00:00:00Z'

  return (
    <div className={`rounded-xl px-5 py-4 border transition-colors ${
      job.enabled
        ? 'bg-[#111827] border-white/10'
        : 'bg-[#111827] border-white/5 opacity-60'
    }`}>
      <div className="flex items-start gap-4">
        <div className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${
          job.enabled ? 'bg-[#22c55e]/10' : 'bg-[#1e293b]'
        }`}>
          {job.enabled ? <Play className="h-4 w-4 text-[#22c55e]" /> : <Pause className="h-4 w-4 text-[#64748b]" />}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h3 className="text-sm font-semibold text-[#f8fafc]">{job.id}</h3>
            <span className="rounded bg-[#1e293b] px-1.5 py-0.5 font-mono text-[10px] text-[#94a3b8]">
              {job.schedule}
            </span>
            <span className="rounded bg-[#1e293b] px-1.5 py-0.5 text-[10px] font-medium text-[#64748b]">
              {job.type}
            </span>
          </div>

          {job.command && (
            <pre className="mt-2 overflow-x-auto rounded-lg bg-[#0c1222] px-3 py-2 font-mono text-xs text-[#94a3b8]">
              {job.command}
            </pre>
          )}

          <div className="mt-2.5 flex flex-wrap items-center gap-3 text-[11px] text-[#64748b]">
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
              <span className="flex items-center gap-1 text-[#f59e0b]">
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
