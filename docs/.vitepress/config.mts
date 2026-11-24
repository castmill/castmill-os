import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, type DefaultTheme } from 'vitepress'

const REPO = 'https://github.com/castmill/castmill-os'
const DOCS_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const REPO_DIR = path.resolve(DOCS_DIR, '..')

// Served at https://os.castmill.com/ (GitHub Pages custom domain). Set
// DOCS_BASE and DOCS_ORIGIN to build for another address, for example
// DOCS_BASE=/castmill-os/ DOCS_ORIGIN=https://<owner>.github.io for a fork's
// project pages.
const base = process.env.DOCS_BASE ?? '/'
// Absolute URL of the site, for link previews (og:image must be absolute).
const SITE_URL = `${process.env.DOCS_ORIGIN ?? 'https://os.castmill.com'}${base}`
const SOCIAL_DESCRIPTION =
  'Open source, immutable and self-healing Linux for digital signage. One signed image that updates itself safely and never needs a visit.'

/**
 * The Markdown files are also read on GitHub, so they link to source files and
 * to pages outside docs/ with relative paths (../agent/..., ../../README.md).
 * Those aren't pages of the site: point them at the file on GitHub instead.
 */
function repoLinks(md: any) {
  md.core.ruler.after('inline', 'castmill-repo-links', (state: any) => {
    const page: string | undefined = state.env.relativePath
    if (!page) return
    for (const block of state.tokens) {
      for (const token of block.children ?? []) {
        if (token.type !== 'link_open') continue
        const href: string | null = token.attrGet('href')
        if (!href || /^([a-z][a-z0-9+.-]*:|#|\/)/i.test(href)) continue
        const [target, hash = ''] = href.split('#')
        if (!target) continue
        const abs = path.resolve(DOCS_DIR, path.dirname(page), target)
        const inDocs = !path.relative(DOCS_DIR, abs).startsWith('..')
        if (inDocs && /\.md$/.test(target)) {
          // README.md is the index of its folder (see rewrites below).
          if (path.basename(target) === 'README.md') {
            token.attrSet('href', target.replace(/README\.md$/, 'index.md') + (hash ? `#${hash}` : ''))
          }
          continue
        }
        if (inDocs && fs.existsSync(path.join(abs, 'README.md'))) {
          token.attrSet('href', `${target.replace(/\/?$/, '/')}index.md${hash ? `#${hash}` : ''}`)
          continue
        }
        const rel = path.relative(REPO_DIR, abs).split(path.sep).join('/')
        const kind = fs.existsSync(abs) && fs.statSync(abs).isDirectory() ? 'tree' : 'blob'
        token.attrSet('href', `${REPO}/${kind}/main/${rel}${hash ? `#${hash}` : ''}`)
      }
    }
  })
}

const specs: DefaultTheme.SidebarItem[] = [
  ['01-vision-and-requirements', '01 · Vision and requirements'],
  ['02-system-architecture', '02 · System architecture'],
  ['03-updates', '03 · Updates and rollouts'],
  ['04-security', '04 · Security'],
  ['05-reliability', '05 · Reliability'],
  ['06-display-and-media', '06 · Display and media'],
  ['07-device-management', '07 · Device management'],
  ['08-installation-and-provisioning', '08 · Installation and provisioning'],
  ['09-hardware-support', '09 · Hardware support'],
  ['10-build-release-testing', '10 · Build, release and testing'],
  ['11-roadmap', '11 · Roadmap'],
].map(([file, text]) => ({ text, link: `/specs/${file}` }))

const adrs: DefaultTheme.SidebarItem[] = fs
  .readdirSync(path.join(DOCS_DIR, 'adr'))
  .filter((f) => /^\d{4}-.*\.md$/.test(f))
  .sort()
  .map((f) => {
    const title = fs.readFileSync(path.join(DOCS_DIR, 'adr', f), 'utf8').match(/^# (.+)$/m)?.[1] ?? f
    return { text: title.replace(/^ADR-(\d+):\s*/, '$1 · '), link: `/adr/${f.replace(/\.md$/, '')}` }
  })

const guideSidebar: DefaultTheme.SidebarItem[] = [
  {
    text: 'Get started',
    items: [
      { text: 'Introduction', link: '/guide/introduction' },
      { text: 'Quick start (Mac)', link: '/guide/quick-start' },
      { text: 'How it works', link: '/guide/how-it-works' },
    ],
  },
  {
    text: 'Install',
    items: [
      { text: 'Overview and downloads', link: '/install/' },
      { text: 'Raspberry Pi 5', link: '/install/raspberry-pi-5' },
      { text: 'Intel mini PCs (x86-64)', link: '/install/intel-x86-64' },
      { text: 'Virtual machine', link: '/install/virtual-machine' },
    ],
  },
  {
    text: 'Set up',
    items: [
      { text: 'Pair with Castmill', link: '/setup/pairing' },
      { text: 'castmill.conf', link: '/setup/castmill-conf' },
    ],
  },
  {
    text: 'Operate',
    items: [
      { text: 'Device schedule', link: '/operate/device-schedule' },
      { text: 'Updates and rollback', link: '/operate/updates' },
      { text: 'Factory reset', link: '/operate/factory-reset' },
      { text: 'Troubleshooting', link: '/operate/troubleshooting' },
    ],
  },
  {
    text: 'Reference',
    items: [
      { text: 'castmill-agent CLI', link: '/reference/agent-cli' },
      { text: 'Castmill integration', link: '/castmill-integration' },
      { text: 'Hardware support', link: '/specs/09-hardware-support' },
      { text: 'Implementation status', link: '/implementation-status' },
    ],
  },
  {
    text: 'Contribute',
    items: [
      { text: 'Development guide', link: '/development' },
      { text: 'Testing a release', link: '/testing' },
      { text: 'Contributing', link: `${REPO}/blob/main/CONTRIBUTING.md` },
    ],
  },
]

export default defineConfig({
  title: 'Castmill OS',
  description:
    'An open source, immutable Linux operating system for digital signage and the Castmill player.',
  lang: 'en-US',
  base,
  cleanUrls: true,
  lastUpdated: true,
  srcExclude: ['**/node_modules/**'],
  rewrites: { 'adr/README.md': 'adr/index.md' },

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }],
    ['meta', { name: 'theme-color', content: '#fd4f00' }],
    // Link previews (Slack, X, LinkedIn, GitHub...). The cover is
    // public/og.png, rendered from .vitepress/social/cover.html.
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'Castmill OS' }],
    ['meta', { property: 'og:title', content: 'Castmill OS' }],
    ['meta', { property: 'og:description', content: SOCIAL_DESCRIPTION }],
    ['meta', { property: 'og:url', content: SITE_URL }],
    ['meta', { property: 'og:image', content: `${SITE_URL}og.png` }],
    ['meta', { property: 'og:image:width', content: '1200' }],
    ['meta', { property: 'og:image:height', content: '630' }],
    ['meta', { property: 'og:image:alt', content: 'Castmill OS: immutable, self-healing Linux for digital signage' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:image', content: `${SITE_URL}og.png` }],
  ],

  markdown: {
    config: repoLinks,
    theme: { light: 'github-light', dark: 'github-dark' },
  },

  vite: {
    // The home page shows the boot logo from the Yocto layer (single source).
    server: { fs: { allow: [REPO_DIR] } },
  },

  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'Castmill OS',

    nav: [
      { text: 'Guide', link: '/guide/introduction', activeMatch: '^/(guide|setup|operate)/' },
      { text: 'Install', link: '/install/', activeMatch: '^/install/' },
      { text: 'Reference', link: '/reference/agent-cli', activeMatch: '^/(reference|castmill-integration)' },
      {
        text: 'Design',
        activeMatch: '^/(specs|adr)/',
        items: [
          { text: 'Specifications', link: '/specs/01-vision-and-requirements' },
          { text: 'Architecture decisions', link: '/adr/' },
          { text: 'Roadmap', link: '/specs/11-roadmap' },
          { text: 'Implementation status', link: '/implementation-status' },
        ],
      },
      {
        text: 'v0.1',
        items: [
          { text: 'Releases', link: `${REPO}/releases` },
          { text: 'Contributing', link: `${REPO}/blob/main/CONTRIBUTING.md` },
          { text: 'Castmill', link: 'https://castmill.io' },
        ],
      },
    ],

    sidebar: {
      '/specs/': [{ text: 'Specifications', items: specs }, { text: 'Architecture decisions', link: '/adr/' }],
      '/adr/': [{ text: 'Architecture decisions', link: '/adr/', items: adrs }, { text: 'Specifications', link: '/specs/01-vision-and-requirements' }],
      '/': guideSidebar,
    },

    socialLinks: [{ icon: 'github', link: REPO }],

    editLink: {
      pattern: `${REPO}/edit/main/docs/:path`,
      text: 'Edit this page on GitHub',
    },

    search: { provider: 'local' },

    outline: { level: [2, 3] },

    footer: {
      message:
        'Documentation licensed under <a href="https://creativecommons.org/licenses/by/4.0/">CC BY 4.0</a>. Code under MIT and Apache-2.0.',
      copyright: '© 2026 The Castmill OS contributors',
    },
  },
})
