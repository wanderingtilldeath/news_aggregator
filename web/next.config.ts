import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  cacheComponents: true,
  partialPrefetching: true,
  reactCompiler: true,
  // Maps that grew past 99 places were split in two (national parks, Ramsar wetlands). The capitals maps were removed: their old
  // addresses go to the library.
  async redirects() {
    return [
      { source: "/maps/national-parks", destination: "/maps/national-parks-north-east", permanent: true },
      { source: "/maps/ramsar", destination: "/maps/ramsar-north-east", permanent: true },
      { source: "/maps/world-capitals", destination: "/maps", permanent: true },
      { source: "/maps/world-capitals-:continent", destination: "/maps", permanent: true },
      { source: "/maps/capitals", destination: "/maps", permanent: true },
      { source: "/maps/mahajanapadas", destination: "/maps", permanent: true },
    ];
  },
};

export default nextConfig;
