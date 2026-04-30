import type { ReactNode } from 'react'
import { Footer, Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import 'nextra-theme-docs/style.css'
import './globals.css'
import { LanguageSwitcher } from '../components/LanguageSwitcher'

export const metadata = {
  title: {
    default: 'Hams',
    template: '%s – Hams',
  },
  description: 'Hams — Declarative IaC for your workstation',
}

export default async function RootLayout({ children }: { children: ReactNode }) {
  const pageMap = await getPageMap()
  const navbar = (
    <Navbar
      logo={<span style={{ fontWeight: 800 }}>Hams 🐹</span>}
      projectLink="https://github.com/zthxxx/hams"
    >
      <LanguageSwitcher />
    </Navbar>
  )
  const footer = <Footer>Hams — Declarative IaC for workstations</Footer>
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head />
      <body>
        <Layout
          pageMap={pageMap}
          docsRepositoryBase="https://github.com/zthxxx/hams/tree/main/docs"
          navbar={navbar}
          footer={footer}
          sidebar={{ defaultMenuCollapseLevel: 2, toggleButton: true }}
          editLink="Edit this page on GitHub →"
          darkMode
        >
          {children}
        </Layout>
      </body>
    </html>
  )
}
