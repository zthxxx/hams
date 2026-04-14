import { useRouter } from 'next/router'
import { useState, useRef, useEffect } from 'react'
import { MdTranslate } from 'react-icons/md'

const locales = [
  { code: 'en', label: 'English' },
  { code: 'zh-CN', label: '简体中文' },
]

export function LanguageSwitcher() {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const currentPath = router.asPath
  const isZhCN = currentPath.startsWith('/zh-CN')
  const currentLocale = isZhCN ? 'zh-CN' : 'en'

  function switchTo(code: string) {
    setOpen(false)
    if (code === currentLocale) return

    // Strip current locale prefix to get the base path
    let basePath = currentPath
    if (isZhCN) {
      basePath = currentPath.replace(/^\/zh-CN/, '') || '/'
    } else if (currentPath.startsWith('/en')) {
      basePath = currentPath.replace(/^\/en/, '') || '/'
    }

    let targetPath: string
    if (code === 'en') {
      // English: landing page at /, docs at /en/docs/...
      targetPath = basePath === '/' ? '/' : `/en${basePath}`
    } else {
      // zh-CN: everything under /zh-CN/
      targetPath = basePath === '/' ? '/zh-CN' : `/zh-CN${basePath}`
    }
    router.push(targetPath)
  }

  return (
    <div ref={ref} style={{ position: 'relative', marginLeft: 8 }}>
      <button
        onClick={() => setOpen(!open)}
        aria-label="Switch language"
        title="Switch language"
        style={{
          display: 'flex',
          alignItems: 'center',
          padding: 8,
          background: 'transparent',
          border: 'none',
          cursor: 'pointer',
          color: 'currentColor',
          borderRadius: 4,
        }}
      >
        <MdTranslate size={20} />
      </button>
      {open && (
        <div
          style={{
            position: 'absolute',
            right: 0,
            top: '100%',
            marginTop: 4,
            minWidth: 140,
            borderRadius: 8,
            border: '1px solid var(--nextra-border-color, #e5e7eb)',
            background: 'var(--nextra-bg, #fff)',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
            zIndex: 50,
            overflow: 'hidden',
          }}
        >
          {locales.map((locale) => (
            <button
              key={locale.code}
              onClick={() => switchTo(locale.code)}
              style={{
                display: 'block',
                width: '100%',
                padding: '8px 16px',
                textAlign: 'left',
                background: locale.code === currentLocale ? 'var(--nextra-primary-hue, #e0f2fe)' : 'transparent',
                border: 'none',
                cursor: 'pointer',
                color: 'currentColor',
                fontSize: 14,
              }}
            >
              {locale.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
