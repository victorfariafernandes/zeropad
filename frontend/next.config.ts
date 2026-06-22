import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NEXT_STATIC_EXPORT === "1" ? { output: "export" } : {}),
  async rewrites() {
    return [{ source: "/_/:path*", destination: "/system/:path*" }];
  },
};

export default nextConfig;
