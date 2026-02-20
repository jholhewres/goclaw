import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { languages } from '@/i18n'
import { Globe } from 'lucide-react'

export function LanguageSwitcher() {
  const { i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const currentLang = languages.find((l) => l.code === i18n.language) || languages[0]

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const changeLanguage = (code: string) => {
    i18n.changeLanguage(code)
    localStorage.setItem('devclaw_language', code)
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-zinc-400 transition-colors hover:bg-white/4 hover:text-zinc-200"
      >
        <Globe className="h-4 w-4" />
        <span className="text-base">{currentLang.flag}</span>
      </button>

      {open && (
        <div className="absolute bottom-full left-0 mb-2 w-36 rounded-xl border border-white/10 bg-dc-dark p-1.5 shadow-xl shadow-black/20">
          {languages.map((lang) => (
            <button
              key={lang.code}
              onClick={() => changeLanguage(lang.code)}
              className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                lang.code === i18n.language
                  ? 'bg-zinc-800 text-zinc-200'
                  : 'text-zinc-400 hover:bg-white/4 hover:text-zinc-200'
              }`}
            >
              <span className="text-base">{lang.flag}</span>
              <span>{lang.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
