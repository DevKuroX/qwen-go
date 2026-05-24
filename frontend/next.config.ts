import type { NextConfig } from 'next';
const config: NextConfig = {
  reactStrictMode: true,
  output: process.env.NEXT_OUTPUT === 'export' ? 'export' : undefined,
  trailingSlash: true,
  transpilePackages: ['@qwenpi/ui', '@qwenpi/types', '@qwenpi/utils'],
};
export default config;
