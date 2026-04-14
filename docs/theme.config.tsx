import { DocsThemeConfig } from 'nextra-theme-docs'
import { LanguageSwitcher } from './components/LanguageSwitcher'

const config: DocsThemeConfig = {
  logo: <span style={{ fontWeight: 800 }}>hams</span>,
  project: { link: 'https://github.com/zthxxx/hams' },
  docsRepositoryBase: 'https://github.com/zthxxx/hams/tree/main/docs',
  footer: { content: 'hams — Declarative IaC for workstations' },
  darkMode: true,
  sidebar: {
    defaultMenuCollapseLevel: Infinity,
    toggleButton: true,
  },
  editLink: {
    content: 'Edit this page on GitHub →',
  },
  navbar: {
    extraContent: <LanguageSwitcher />,
  },
}

export default config
