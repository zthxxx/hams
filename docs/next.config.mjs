import nextra from 'nextra'

const withNextra = nextra({
  search: { codeblocks: false },
  contentDirBasePath: '/',
})

export default withNextra({
  output: 'export',
  distDir: 'dist',
  images: { unoptimized: true },
  reactStrictMode: true,
  trailingSlash: true,
})
