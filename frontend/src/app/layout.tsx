import type { Metadata } from "next";
import { Outfit, Playfair_Display } from "next/font/google";
import "./globals.css";

const outfit = Outfit({
  subsets: ["latin"],
  variable: "--font-outfit",
  weight: ["300", "400", "500", "600", "700"],
});

const playfair = Playfair_Display({
  subsets: ["latin"],
  variable: "--font-playfair",
  weight: ["400", "600", "700"],
  style: ["normal", "italic"],
});

export const metadata: Metadata = {
  title: "Aura Oasis - Premium In-Room Experience",
  description: "Curated culinary experiences delivered straight to your private sanctuary.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${outfit.variable} ${playfair.variable} h-full antialiased`}
    >
      <body className="min-h-full bg-obsidian-950 text-zinc-100 font-sans flex flex-col selection:bg-gold-500 selection:text-obsidian-950">
        {children}
      </body>
    </html>
  );
}
