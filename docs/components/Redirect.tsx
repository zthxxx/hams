import { useEffect } from 'react'
import { useRouter } from 'next/router'
import Head from 'next/head'

export function Redirect({ to }: { to: string }) {
  const router = useRouter()
  useEffect(() => {
    router.replace(to)
  }, [router, to])
  return (
    <Head>
      <meta httpEquiv="refresh" content={`0;url=${to}`} />
    </Head>
  )
}
