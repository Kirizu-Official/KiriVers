import { defineConfig, type DefaultTheme } from 'vitepress'

function mermaidFence(md: { renderer: { rules: Record<string, Function | undefined> } }) {
  const fence = md.renderer.rules.fence
  if (!fence) return
  md.renderer.rules.fence = (...args) => {
    const [tokens, idx] = args
    const token = tokens[idx]
    if (token.info.trim() === 'mermaid') {
      return `<Mermaid code="${encodeURIComponent(token.content)}" />\n`
    }
    return fence(...args)
  }
}

/** Keep `{project_ref}` and similar path templates from being compiled as Vue interpolations. */
function escapeVueMustaches(md: { renderer: { rules: Record<string, Function | undefined> }; utils: { escapeHtml: (s: string) => string } }) {
  const replaceBraces = (html: string) => html.replace(/\{/g, '&#123;').replace(/\}/g, '&#125;')
  const wrap = (name: string) => {
    const orig = md.renderer.rules[name]
    if (!orig) return
    md.renderer.rules[name] = (...args: unknown[]) => replaceBraces(orig(...args))
  }
  const text = md.renderer.rules.text
  md.renderer.rules.text = (...args: unknown[]) => {
    const html = text ? text(...args) : md.utils.escapeHtml((args[0] as { content: string }[])[args[1] as number].content)
    return replaceBraces(html)
  }
  wrap('code_inline')
  wrap('fence')
  wrap('code_block')
}

function zhGuide(): DefaultTheme.SidebarItem[] {
  return [
    { text: '简介', link: '/guide/' },
    {
      text: '安装',
      collapsed: false,
      items: [
        { text: '选哪种安装方式', link: '/guide/install/' },
        { text: '一键安装', link: '/guide/install/one-click' },
        { text: '手动安装', link: '/guide/install/manual' },
        { text: 'Docker 部署', link: '/guide/install/docker' },
        { text: 'Docker Compose', link: '/guide/install/compose' },
      ],
    },
    { text: 'CLI', link: '/guide/cli' },
    {
      text: '配置',
      collapsed: false,
      items: [
        { text: '配置说明', link: '/guide/config/' },
        { text: '管理员初始化', link: '/guide/config/bootstrap-admin' },
        { text: '客户端访问配置', link: '/guide/config/client.yaml' },
        { text: '服务端配置', link: '/guide/config/config.yaml' },
        { text: '缓存与 Redis', link: '/guide/config/cache' },
        { text: '远程存储（S3）', link: '/guide/config/s3' },
        { text: '本机磁盘存储', link: '/guide/config/local-storage' },
        { text: '管理后台配置', link: '/guide/config/admin.yaml' },
        { text: '反向代理与 TLS', link: '/guide/config/reverse-proxy' },
        { text: '多节点', link: '/guide/config/cluster' },
        { text: '安全相关配置', link: '/guide/config/security' },
      ],
    },
    { text: '备份与升级', link: '/guide/backup' },
    {
      text: '功能指南',
      collapsed: false,
      items: [
        { text: '总览', link: '/guide/features/' },
        { text: '检查更新', link: '/guide/features/check' },
        { text: '渠道', link: '/guide/features/channels' },
        { text: '版本发布', link: '/guide/features/versions' },
        { text: '灰度发布', link: '/guide/features/gray' },
        { text: '增量更新', link: '/guide/features/incremental' },
        { text: '公告', link: '/guide/features/announcements' },
        { text: '更新说明', link: '/guide/features/changelog' },
        { text: '安装策略', link: '/guide/features/install-policy' },
        { text: '平台矩阵', link: '/guide/features/matrix' },
        { text: '硬件代号', link: '/guide/features/hw-revs' },
        { text: '访问密钥', link: '/guide/features/tokens' },
        { text: '商店订阅', link: '/guide/features/store' },
        { text: '遥测与设备', link: '/guide/features/telemetry' },
        { text: '多节点', link: '/guide/features/cluster' },
        { text: 'CDN 与下载', link: '/guide/features/cdn' },
        { text: 'Webhook', link: '/guide/features/webhooks' },
        { text: '语言', link: '/guide/features/languages' },
      ],
    },
    {
      text: '使用场景',
      collapsed: false,
      items: [
        { text: '场景总览', link: '/guide/scenarios/' },
        { text: '桌面应用', link: '/guide/scenarios/desktop' },
        { text: 'Electron', link: '/guide/scenarios/electron' },
        { text: 'Tauri', link: '/guide/scenarios/tauri' },
        { text: 'macOS / Sparkle', link: '/guide/scenarios/sparkle' },
        { text: '其他桌面更新器', link: '/guide/scenarios/desktop-feeds' },
        { text: '移动应用', link: '/guide/scenarios/mobile' },
        { text: 'MCU / 固件', link: '/guide/scenarios/mcu' },
        { text: '服务与命令行程序', link: '/guide/scenarios/server-cli' },
        { text: '网站 / Node', link: '/guide/scenarios/web-node' },
      ],
    },
  ]
}

function zhAdmin(): DefaultTheme.SidebarItem[] {
  return [
    { text: '快速上手', link: '/admin/quick-start' },
    { text: '登录与两步验证', link: '/admin/security' },
    { text: '用户管理', link: '/admin/admins' },
    { text: '节点管理', link: '/admin/nodes' },
    { text: 'GeoIP', link: '/admin/geoip' },
    {
      text: '项目管理',
      collapsed: false,
      items: [
        { text: '项目创建', link: '/admin/projects/create' },
        { text: '项目概览', link: '/admin/projects/overview' },
        { text: '项目设置', link: '/admin/projects/settings' },
        { text: '安装策略', link: '/admin/projects/install-policy' },
        { text: '设备列表', link: '/admin/projects/clients' },
        { text: '渠道', link: '/admin/projects/channels' },
        { text: '平台矩阵', link: '/admin/projects/matrix' },
        { text: '硬件代号', link: '/admin/projects/hw-revs' },
        { text: '语言', link: '/admin/projects/languages' },
        { text: '版本', link: '/admin/projects/versions' },
        { text: '灰度发布', link: '/admin/projects/gray' },
        { text: '公告', link: '/admin/projects/announcements' },
        { text: '访问密钥', link: '/admin/projects/tokens' },
        { text: '批量上传发版', link: '/admin/projects/release' },
        { text: '操作记录', link: '/admin/projects/audit' },
      ],
    },
    { text: '后台任务', link: '/admin/jobs' },
  ]
}

function zhApi(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'SDK',
      collapsed: false,
      items: [
        { text: '总览', link: '/api/sdk/' },
        { text: '概念', link: '/api/sdk/concepts' },
        { text: 'Python', link: '/api/sdk/python' },
        { text: 'Java', link: '/api/sdk/java' },
        { text: 'Kotlin', link: '/api/sdk/kotlin' },
        { text: 'C#', link: '/api/sdk/csharp' },
        { text: 'TypeScript', link: '/api/sdk/typescript' },
        { text: 'Rust', link: '/api/sdk/rust' },
        { text: 'Dart', link: '/api/sdk/dart' },
        { text: 'C', link: '/api/sdk/c' },
        { text: 'C++', link: '/api/sdk/cpp' },
        { text: 'Go', link: '/api/sdk/go' },
        { text: 'PHP', link: '/api/sdk/php' },
        { text: 'Swift', link: '/api/sdk/swift' },
      ],
    },
    { text: '调用约定', link: '/api/conventions' },
    {
      text: '客户端调用',
      collapsed: false,
      items: [
        { text: '总览', link: '/api/client/' },
        { text: '探活', link: '/api/client/health' },
        { text: '项目公开信息', link: '/api/client/project' },
        { text: '设备上报', link: '/api/client/report' },
        { text: '公开目录', link: '/api/client/catalog' },
        { text: '检查更新', link: '/api/client/check' },
        { text: '更新说明', link: '/api/client/changelog' },
        { text: '下载', link: '/api/client/packages' },
        { text: '完整性清单', link: '/api/client/integrity' },
        { text: '单文件差量', link: '/api/client/diff' },
        { text: '多文件打包', link: '/api/client/pack' },
        { text: '遥测', link: '/api/client/telemetry' },
        { text: '公告', link: '/api/client/announcements' },
        { text: '配图', link: '/api/client/media' },
        { text: '商店 feed', link: '/api/client/store' },
      ],
    },
    {
      text: '管理后台自动化',
      collapsed: false,
      items: [
        { text: '总览', link: '/api/admin/' },
        { text: '鉴权', link: '/api/admin/auth' },
        { text: '项目', link: '/api/admin/projects' },
        { text: '版本', link: '/api/admin/versions' },
      ],
    },
    { text: '自动发版', link: '/api/ci' },
    { text: 'API 参考', link: '/api/reference/' },
  ]
}

function zhFaq(): DefaultTheme.SidebarItem[] {
  return [
    { text: '目录', link: '/faq/' },
    { text: '安装不上 / 打不开网页', link: '/faq/install' },
    { text: '管理后台', link: '/faq/admin' },
    { text: '用户端没有更新', link: '/faq/client' },
    { text: '下载与差量', link: '/faq/delta' },
    { text: '商店订阅', link: '/faq/store' },
    { text: '运维', link: '/faq/ops' },
    { text: '失败代码对照', link: '/faq/errors' },
  ]
}

function zhContribute(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: '参与',
      collapsed: false,
      items: [
        { text: '总览', link: '/contribute/' },
        { text: '提交 BUG', link: '/contribute/bugs' },
        { text: '贡献代码', link: '/contribute/pull-requests' },
      ],
    },
    {
      text: '仓库与模型',
      collapsed: false,
      items: [
        { text: '目录结构', link: '/contribute/repo-layout' },
        { text: '运行架构', link: '/contribute/architecture' },
        { text: '技术白皮书', link: '/contribute/whitepaper' },
      ],
    },
    {
      text: '开发循环',
      collapsed: false,
      items: [
        { text: '本地开发', link: '/contribute/dev-setup' },
        { text: '发版与 Docker', link: '/contribute/release' },
        { text: '测试', link: '/contribute/testing' },
        { text: 'OpenAPI 契约', link: '/contribute/openapi' },
      ],
    },
    {
      text: '代码分层',
      collapsed: false,
      items: [
        { text: '后端分层', link: '/contribute/backend' },
        { text: '管理台前端', link: '/contribute/frontend' },
      ],
    },
    {
      text: '子系统',
      collapsed: false,
      items: [
        { text: '差量引擎', link: '/contribute/delta' },
        { text: '商店适配器', link: '/contribute/store' },
        { text: 'SDK 源码分支', link: '/contribute/sdk' },
      ],
    },
  ]
}

function enGuide(): DefaultTheme.SidebarItem[] {
  return [
    { text: 'Introduction', link: '/en/guide/' },
    {
      text: 'Install',
      collapsed: false,
      items: [
        { text: 'Choose an install path', link: '/en/guide/install/' },
        { text: 'One-click dependencies', link: '/en/guide/install/one-click' },
        { text: 'Manual install', link: '/en/guide/install/manual' },
        { text: 'Docker', link: '/en/guide/install/docker' },
        { text: 'Docker Compose', link: '/en/guide/install/compose' },
      ],
    },
    { text: 'CLI', link: '/en/guide/cli' },
    {
      text: 'Configuration',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/guide/config/' },
        { text: 'Bootstrap the first admin', link: '/en/guide/config/bootstrap-admin' },
        { text: 'client.yaml', link: '/en/guide/config/client.yaml' },
        { text: 'config.yaml', link: '/en/guide/config/config.yaml' },
        { text: 'Cache and Redis', link: '/en/guide/config/cache' },
        { text: 'Remote storage (S3)', link: '/en/guide/config/s3' },
        { text: 'Local disk storage', link: '/en/guide/config/local-storage' },
        { text: 'admin.yaml', link: '/en/guide/config/admin.yaml' },
        { text: 'Reverse proxy and TLS', link: '/en/guide/config/reverse-proxy' },
        { text: 'Cluster', link: '/en/guide/config/cluster' },
        { text: 'Security settings', link: '/en/guide/config/security' },
      ],
    },
    { text: 'Backup and upgrade', link: '/en/guide/backup' },
    {
      text: 'Feature guides',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/guide/features/' },
        { text: 'Update check', link: '/en/guide/features/check' },
        { text: 'Channels', link: '/en/guide/features/channels' },
        { text: 'Releases', link: '/en/guide/features/versions' },
        { text: 'Gray rollout', link: '/en/guide/features/gray' },
        { text: 'Incremental updates', link: '/en/guide/features/incremental' },
        { text: 'Announcements', link: '/en/guide/features/announcements' },
        { text: 'Changelog', link: '/en/guide/features/changelog' },
        { text: 'Install policy', link: '/en/guide/features/install-policy' },
        { text: 'Platform matrix', link: '/en/guide/features/matrix' },
        { text: 'Hardware revisions', link: '/en/guide/features/hw-revs' },
        { text: 'Access tokens', link: '/en/guide/features/tokens' },
        { text: 'Store listings', link: '/en/guide/features/store' },
        { text: 'Telemetry and devices', link: '/en/guide/features/telemetry' },
        { text: 'Multi-node', link: '/en/guide/features/cluster' },
        { text: 'CDN and downloads', link: '/en/guide/features/cdn' },
        { text: 'Webhooks', link: '/en/guide/features/webhooks' },
        { text: 'Languages', link: '/en/guide/features/languages' },
      ],
    },
    {
      text: 'Scenarios',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/guide/scenarios/' },
        { text: 'Desktop apps', link: '/en/guide/scenarios/desktop' },
        { text: 'Electron', link: '/en/guide/scenarios/electron' },
        { text: 'Tauri', link: '/en/guide/scenarios/tauri' },
        { text: 'macOS / Sparkle', link: '/en/guide/scenarios/sparkle' },
        { text: 'Other desktop updaters', link: '/en/guide/scenarios/desktop-feeds' },
        { text: 'Mobile', link: '/en/guide/scenarios/mobile' },
        { text: 'MCU / firmware', link: '/en/guide/scenarios/mcu' },
        { text: 'Servers and CLI', link: '/en/guide/scenarios/server-cli' },
        { text: 'Web / Node', link: '/en/guide/scenarios/web-node' },
      ],
    },
  ]
}

function enAdmin(): DefaultTheme.SidebarItem[] {
  return [
    { text: 'Quick start', link: '/en/admin/quick-start' },
    { text: 'Sign-in and 2FA', link: '/en/admin/security' },
    { text: 'Admins', link: '/en/admin/admins' },
    { text: 'Nodes', link: '/en/admin/nodes' },
    { text: 'GeoIP', link: '/en/admin/geoip' },
    {
      text: 'Projects',
      collapsed: false,
      items: [
        { text: 'Create a project', link: '/en/admin/projects/create' },
        { text: 'Overview', link: '/en/admin/projects/overview' },
        { text: 'Settings', link: '/en/admin/projects/settings' },
        { text: 'Install policy', link: '/en/admin/projects/install-policy' },
        { text: 'Clients', link: '/en/admin/projects/clients' },
        { text: 'Channels', link: '/en/admin/projects/channels' },
        { text: 'Platform matrix', link: '/en/admin/projects/matrix' },
        { text: 'Hardware revisions', link: '/en/admin/projects/hw-revs' },
        { text: 'Languages', link: '/en/admin/projects/languages' },
        { text: 'Versions', link: '/en/admin/projects/versions' },
        { text: 'Gray rollout', link: '/en/admin/projects/gray' },
        { text: 'Announcements', link: '/en/admin/projects/announcements' },
        { text: 'Tokens', link: '/en/admin/projects/tokens' },
        { text: 'Batch release', link: '/en/admin/projects/release' },
        { text: 'Audit log', link: '/en/admin/projects/audit' },
      ],
    },
    { text: 'Jobs', link: '/en/admin/jobs' },
  ]
}

function enApi(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'SDK',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/api/sdk/' },
        { text: 'Concepts', link: '/en/api/sdk/concepts' },
        { text: 'Python', link: '/en/api/sdk/python' },
        { text: 'Java', link: '/en/api/sdk/java' },
        { text: 'Kotlin', link: '/en/api/sdk/kotlin' },
        { text: 'C#', link: '/en/api/sdk/csharp' },
        { text: 'TypeScript', link: '/en/api/sdk/typescript' },
        { text: 'Rust', link: '/en/api/sdk/rust' },
        { text: 'Dart', link: '/en/api/sdk/dart' },
        { text: 'C', link: '/en/api/sdk/c' },
        { text: 'C++', link: '/en/api/sdk/cpp' },
        { text: 'Go', link: '/en/api/sdk/go' },
        { text: 'PHP', link: '/en/api/sdk/php' },
        { text: 'Swift', link: '/en/api/sdk/swift' },
      ],
    },
    { text: 'Conventions', link: '/en/api/conventions' },
    {
      text: 'Client HTTP',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/api/client/' },
        { text: 'Health', link: '/en/api/client/health' },
        { text: 'Public project', link: '/en/api/client/project' },
        { text: 'Device report', link: '/en/api/client/report' },
        { text: 'Catalogs', link: '/en/api/client/catalog' },
        { text: 'Update check', link: '/en/api/client/check' },
        { text: 'Changelog', link: '/en/api/client/changelog' },
        { text: 'Downloads', link: '/en/api/client/packages' },
        { text: 'Integrity', link: '/en/api/client/integrity' },
        { text: 'Single-file diff', link: '/en/api/client/diff' },
        { text: 'Multi-file pack', link: '/en/api/client/pack' },
        { text: 'Telemetry', link: '/en/api/client/telemetry' },
        { text: 'Announcements', link: '/en/api/client/announcements' },
        { text: 'Media', link: '/en/api/client/media' },
        { text: 'Store feeds', link: '/en/api/client/store' },
      ],
    },
    {
      text: 'Admin automation',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/api/admin/' },
        { text: 'Auth', link: '/en/api/admin/auth' },
        { text: 'Projects', link: '/en/api/admin/projects' },
        { text: 'Versions', link: '/en/api/admin/versions' },
      ],
    },
    { text: 'CI releases', link: '/en/api/ci' },
    { text: 'API reference', link: '/en/api/reference/' },
  ]
}

function enFaq(): DefaultTheme.SidebarItem[] {
  return [
    { text: 'Index', link: '/en/faq/' },
    { text: 'Install / console not opening', link: '/en/faq/install' },
    { text: 'Admin console', link: '/en/faq/admin' },
    { text: 'Client sees no update', link: '/en/faq/client' },
    { text: 'Download and delta', link: '/en/faq/delta' },
    { text: 'Store listings', link: '/en/faq/store' },
    { text: 'Operations', link: '/en/faq/ops' },
    { text: 'Error codes', link: '/en/faq/errors' },
  ]
}

function enContribute(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'Participate',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/en/contribute/' },
        { text: 'Report a bug', link: '/en/contribute/bugs' },
        { text: 'Pull requests', link: '/en/contribute/pull-requests' },
      ],
    },
    {
      text: 'Repository and model',
      collapsed: false,
      items: [
        { text: 'Repository layout', link: '/en/contribute/repo-layout' },
        { text: 'Runtime architecture', link: '/en/contribute/architecture' },
        { text: 'Technical whitepaper', link: '/en/contribute/whitepaper' },
      ],
    },
    {
      text: 'Development loop',
      collapsed: false,
      items: [
        { text: 'Local development', link: '/en/contribute/dev-setup' },
        { text: 'Release and Docker', link: '/en/contribute/release' },
        { text: 'Testing', link: '/en/contribute/testing' },
        { text: 'OpenAPI contracts', link: '/en/contribute/openapi' },
      ],
    },
    {
      text: 'Code layers',
      collapsed: false,
      items: [
        { text: 'Backend layers', link: '/en/contribute/backend' },
        { text: 'Admin frontend', link: '/en/contribute/frontend' },
      ],
    },
    {
      text: 'Subsystems',
      collapsed: false,
      items: [
        { text: 'Delta engines', link: '/en/contribute/delta' },
        { text: 'Store adapters', link: '/en/contribute/store' },
        { text: 'SDK source branches', link: '/en/contribute/sdk' },
      ],
    },
  ]
}

export default defineConfig({
  srcDir: 'docs',
  lang: 'zh-CN',
  title: 'KiriVers',
  description: '自托管软件更新服务',
  lastUpdated: true,
  cleanUrls: true,
  ignoreDeadLinks: false,
  head: [['link', { rel: 'icon', href: '/logo.svg' }]],
  markdown: {
    lineNumbers: true,
    config(md) {
      mermaidFence(md)
      escapeVueMustaches(md as { renderer: { rules: Record<string, Function | undefined> }; utils: { escapeHtml: (s: string) => string } })
    },
  },
  vite: {
    optimizeDeps: {
      include: ['mermaid'],
    },
    ssr: {
      noExternal: ['mermaid'],
      external: ['@scalar/api-reference'],
    },
  },
  themeConfig: {
    logo: '/logo.svg',
    search: {
      provider: 'local',
      options: {
        locales: {
          root: {
            translations: {
              button: { buttonText: '搜索', buttonAriaLabel: '搜索文档' },
              modal: {
                noResultsText: '没有匹配结果',
                resetButtonTitle: '清除查询',
                footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' },
              },
            },
          },
          en: {
            translations: {
              button: { buttonText: 'Search', buttonAriaLabel: 'Search docs' },
              modal: {
                noResultsText: 'No results',
                resetButtonTitle: 'Clear query',
                footer: { selectText: 'to select', navigateText: 'to navigate', closeText: 'to close' },
              },
            },
          },
        },
      },
    },
    socialLinks: [{ icon: 'github', link: 'https://github.com/Kirizu-Official/KiriVers' }],
  },
  locales: {
    root: {
      label: '简体中文',
      lang: 'zh-CN',
      title: 'KiriVers',
      description: '自托管软件更新服务',
      themeConfig: {
        nav: [
          { text: '指南', link: '/guide/' },
          { text: '管理', link: '/admin/quick-start' },
          { text: 'API', link: '/api/sdk/' },
          { text: 'FAQ', link: '/faq/' },
          { text: '贡献', link: '/contribute/' },
        ],
        sidebar: {
          '/guide/': zhGuide(),
          '/admin/': zhAdmin(),
          '/api/': zhApi(),
          '/faq/': zhFaq(),
          '/contribute/': zhContribute(),
        },
        outline: { label: '本页目录' },
        docFooter: { prev: '上一页', next: '下一页' },
        lastUpdated: { text: '上次更新' },
        returnToTopLabel: '回到顶部',
        sidebarMenuLabel: '菜单',
        darkModeSwitchLabel: '外观',
        langMenuLabel: '语言',
      },
    },
    en: {
      label: 'English',
      lang: 'en-US',
      link: '/en/',
      title: 'KiriVers',
      description: 'Self-hosted software update service',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/en/guide/' },
          { text: 'Admin', link: '/en/admin/quick-start' },
          { text: 'API', link: '/en/api/sdk/' },
          { text: 'FAQ', link: '/en/faq/' },
          { text: 'Contribute', link: '/en/contribute/' },
        ],
        sidebar: {
          '/en/guide/': enGuide(),
          '/en/admin/': enAdmin(),
          '/en/api/': enApi(),
          '/en/faq/': enFaq(),
          '/en/contribute/': enContribute(),
        },
      },
    },
  },
})
