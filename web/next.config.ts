import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  cacheComponents: true,
  partialPrefetching: true,
  reactCompiler: true,
  // Maps that grew past 99 places were split in two (national parks, Ramsar wetlands) or by continent (world capitals).
  async redirects() {
    return [
      { source: "/maps/national-parks", destination: "/maps/national-parks-north-east", permanent: true },
      { source: "/maps/ramsar", destination: "/maps/ramsar-north-east", permanent: true },
      { source: "/maps/world-capitals", destination: "/maps/world-capitals-asia", permanent: true },
    ];
  },
};

export default nextConfig;
