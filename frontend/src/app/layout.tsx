import type { Metadata } from "next";
import { JetBrains_Mono, Outfit } from "next/font/google";
import AppProviders from "@/components/providers/AppProviders";
import "./globals.css";

const displaySans = Outfit({
  variable: "--font-display-sans",
  subsets: ["latin"],
});

const mono = JetBrains_Mono({
  variable: "--font-display-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Stratum Control",
  description: "Realtime torrent control plane",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${displaySans.variable} ${mono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <AppProviders>{children}</AppProviders>
      </body>
    </html>
  );
}
