// Getting Started, Usage and Features share one flat, always-open sidebar so
// the top bar stays short enough to fit on a phone.
const DOCS = [
  {
    text: 'Getting Started',
    items: [
      { text: 'Overview', link: '/getting-started/' },
      { text: 'Requirements', link: '/getting-started/requirements' },
      { text: 'Installation', link: '/getting-started/installation' },
      { text: 'Quick Start', link: '/getting-started/quick-start' },
      { text: 'Updating from before 1.26', link: '/getting-started/updating-from-pre-1.26' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'Uninstall', link: '/getting-started/uninstall' },
    ],
  },
  {
    text: 'Platform guides',
    items: [
      { text: 'Omarchy', link: '/getting-started/omarchy' },
      { text: 'NixOS', link: '/getting-started/nixos' },
      { text: 'Windows', link: '/getting-started/windows' },
      { text: 'Windows (WSL2) in depth', link: '/getting-started/wsl2' },
    ],
  },
  {
    text: 'Coming from another tool',
    items: [
      { text: 'Comparison', link: '/getting-started/comparison' },
      { text: 'Laravel Herd for Linux', link: '/getting-started/herd-linux' },
      { text: 'Laragon for Linux', link: '/getting-started/laragon-linux' },
      { text: 'Moving from MAMP', link: '/getting-started/mamp' },
      { text: 'Databases from DBngin or Herd', link: '/getting-started/moving-databases' },
      { text: 'XAMPP alternative', link: '/getting-started/xampp' },
      { text: 'Laradock alternative', link: '/getting-started/laradock' },
      { text: 'Laravel Sail alternative', link: '/getting-started/sail' },
      { text: 'Lerd vs DDEV', link: '/getting-started/lerd-vs-ddev' },
      { text: 'Lerd vs Lando', link: '/getting-started/lerd-vs-lando' },
    ],
  },
  {
    text: 'Add-ons',
    items: [
      { text: 'Services (MongoDB, phpMyAdmin, …)', link: '/getting-started/services' },
      { text: 'NativePHP (desktop & mobile)', link: '/getting-started/nativephp' },
    ],
  },
  {
    text: 'Lifecycle',
    items: [
      { text: 'Start, Stop & Autostart', link: '/usage/lifecycle' },
    ],
  },
  {
    text: 'Sites & Runtimes',
    items: [
      { text: 'Site Management', link: '/usage/sites' },
      { text: 'Domains', link: '/usage/domains' },
      { text: 'Site Groups', link: '/usage/site-groups' },
      { text: 'Workspaces', link: '/usage/workspaces' },
      { text: 'Pausing Sites', link: '/usage/pausing' },
      { text: 'PHP', link: '/usage/php' },
      { text: 'PHP versions', link: '/usage/php-versions' },
      { text: 'PHP extensions', link: '/usage/php-extensions' },
      { text: 'php.ini overrides', link: '/usage/php-ini' },
      { text: 'Xdebug', link: '/usage/xdebug' },
      { text: 'Composer', link: '/usage/composer' },
      { text: 'Node', link: '/usage/node' },
      { text: 'Custom Containers', link: '/usage/custom-containers' },
      { text: 'Host-Proxy Sites', link: '/usage/host-proxy' },
      { text: 'Nginx Overrides', link: '/usage/nginx-overrides' },
    ],
  },
  {
    text: 'Services & Data',
    items: [
      { text: 'Services', link: '/usage/services' },
      { text: 'Service updates', link: '/usage/service-updates' },
      { text: 'Service presets', link: '/usage/service-presets' },
      { text: 'Custom services', link: '/usage/custom-services' },
      { text: 'Database', link: '/usage/database' },
      { text: 'Disk cleanup', link: '/usage/cleanup' },
    ],
  },
  {
    text: 'Frameworks & Workers',
    items: [
      { text: 'Frameworks', link: '/usage/frameworks' },
      { text: 'Framework Workers', link: '/usage/framework-workers' },
      { text: 'Framework Commands', link: '/usage/framework-commands' },
      { text: 'Framework Definitions', link: '/usage/framework-definitions' },
      { text: 'Queue Workers', link: '/usage/queue-workers' },
      { text: 'Idle-Suspend', link: '/usage/idle-suspend' },
      { text: 'Worker Runtime (macOS)', link: '/usage/worker-runtime' },
      { text: 'Healing Failed Workers', link: '/usage/worker-heal' },
      { text: 'Browser Testing', link: '/usage/browser-testing' },
    ],
  },
  {
    text: 'Integrations & Migration',
    items: [
      { text: 'Stripe', link: '/usage/stripe' },
      { text: 'Sharing Sites', link: '/usage/sharing' },
      { text: 'LAN sharing', link: '/usage/lan-sharing' },
      { text: 'Remote / LAN Development', link: '/usage/remote-development' },
      { text: 'Importing from Sail', link: '/usage/import-sail' },
    ],
  },
  {
    text: 'Interfaces',
    items: [
      { text: 'Features overview', link: '/features/' },
      { text: 'Web UI', link: '/features/web-ui' },
      { text: 'Dashboard themes', link: '/features/dashboard-themes' },
      { text: 'Terminal Dashboard', link: '/features/tui' },
      { text: 'System Tray', link: '/features/system-tray' },
      { text: 'Omarchy Bar Widget', link: '/features/omarchy-glance' },
      { text: 'PhpStorm Plugin', link: '/features/phpstorm-plugin' },
      { text: 'AI Integration (MCP)', link: '/features/mcp' },
      { text: 'Community Plugins', link: '/features/plugins' },
    ],
  },
  {
    text: 'Serving sites',
    items: [
      { text: 'HTTPS / TLS', link: '/features/https' },
      { text: 'DNS', link: '/features/dns' },
      { text: 'Project Setup', link: '/features/project-setup' },
      { text: 'Environment Setup', link: '/features/env-setup' },
      { text: 'Git Worktrees', link: '/features/git-worktrees' },
      { text: 'FrankenPHP runtime', link: '/features/frankenphp' },
      { text: 'Native runtime', link: '/features/native-runtime' },
    ],
  },
  {
    text: 'Inspecting & debugging',
    items: [
      { text: 'Logs', link: '/features/logs' },
      { text: 'Dump viewer', link: '/features/dumps' },
      { text: 'Query viewer', link: '/features/queries' },
      { text: 'Browser logs', link: '/features/browser-logs' },
      { text: 'Request Timing', link: '/features/request-timing' },
      { text: 'Profiler', link: '/features/profiler' },
      { text: 'Tinker tab', link: '/features/tinker' },
      { text: 'Notifications', link: '/features/notifications' },
    ],
  },
  {
    text: 'Framework walkthroughs',
    items: [
      { text: 'Laravel', link: '/getting-started/laravel' },
      { text: 'Symfony', link: '/getting-started/symfony' },
      { text: 'WordPress', link: '/getting-started/wordpress' },
      { text: 'Drupal', link: '/getting-started/drupal' },
      { text: 'TYPO3', link: '/getting-started/typo3' },
      { text: 'CakePHP', link: '/getting-started/cakephp' },
      { text: 'CodeIgniter', link: '/getting-started/codeigniter' },
      { text: 'Statamic', link: '/getting-started/statamic' },
      { text: 'Winter CMS', link: '/getting-started/winter' },
      { text: 'Bedrock', link: '/getting-started/bedrock' },
      { text: 'Lumen', link: '/getting-started/lumen' },
      { text: 'Yii', link: '/getting-started/yii' },
      { text: 'Tempest', link: '/getting-started/tempest' },
      { text: 'Magento', link: '/getting-started/magento' },
      { text: 'Containers (Node, Python, Go, …)', link: '/getting-started/containers' },
    ],
  },
]

const REFERENCE = [
  {
    text: 'Reference',
    items: [
      { text: 'Overview', link: '/reference/' },
      { text: 'Command Reference', link: '/reference/commands' },
      { text: 'Configuration', link: '/configuration' },
    ],
  },
  {
    text: 'Internals',
    items: [
      { text: 'Directory Layout', link: '/reference/directory-layout' },
      { text: 'Architecture', link: '/reference/architecture' },
    ],
  },
]

export const SIDEBAR = {
  '/getting-started/': DOCS,
  '/usage/': DOCS,
  '/features/': DOCS,
  '/configuration': [
    {
      text: 'Configuration',
      items: [
        { text: 'Overview', link: '/configuration' },
        { text: 'Per-project (.lerd.yaml)', link: '/configuration#per-project-config-lerd-yaml' },
      ],
    },
  ],
  '/reference/': REFERENCE,
  '/troubleshooting': DOCS,
  '/contributing/': [
    {
      text: 'Contributing',
      items: [
        { text: 'Overview', link: '/contributing/' },
        { text: 'Building from Source', link: '/contributing/building' },
        { text: 'Pull Requests', link: '/contributing/pull-requests' },
        { text: 'Extending the Plugin System', link: '/contributing/plugins' },
        { text: 'Measuring Performance', link: '/contributing/benchmark' },
      ],
    },
  ],
}
