// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

import cloudflare from '@astrojs/cloudflare';

const site = process.env.SITE_URL || undefined;

export default defineConfig({
  site,

  integrations: [
    starlight({
      title: 'asana-cli',
      description: 'A JSON-first CLI for Asana, designed for agents and scripts.',
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/thaodangspace/asana-cli',
        },
      ],
      sidebar: [
        {
          label: 'Start here',
          items: [
            { label: 'Overview', slug: '' },
            { label: 'Getting started', slug: 'getting-started' },
          ],
        },
        {
          label: 'Using asana-cli',
          items: [
            { label: 'Commands', slug: 'commands' },
            { label: 'Custom fields', slug: 'custom-fields' },
            { label: 'Output and errors', slug: 'output-and-errors' },
            { label: 'Configuration and security', slug: 'configuration-security' },
          ],
        },
        {
          label: 'Reference',
          items: [{ label: 'Deploy to Cloudflare Pages', slug: 'deploy' }],
        },
        {
          label: 'Maintainers',
          items: [
            { label: 'Releasing', slug: 'maintainers/releasing' },
            { label: 'Repository rules', slug: 'maintainers/repository-rules' },
          ],
        },
      ],
      customCss: ['./src/styles/custom.css'],
    }),
  ],

  adapter: cloudflare(),
});
