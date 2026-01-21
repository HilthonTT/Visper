import { Link } from "@tanstack/react-router";

const NAV_LINKS = [
  {
    path: "/",
    label: "home",
    symbol: "~",
  },
  {
    path: "/faq",
    label: "faq",
    symbol: "?",
  },
  {
    path: "/api",
    label: "api",
    symbol: "{",
  },
];

export function Navbar() {
  return (
    <nav className="sticky top-0 z-50 border-b border-[#2D3748] bg-[#0A0E14]/95 backdrop-blur supports-backdrop-filter:bg-[#0A0E14]/90">
      <div className="container mx-auto px-4">
        <div className="flex h-14 items-center justify-between font-mono text-sm">
          {/* Logo/Brand */}
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2 text-[#3B82F6]">
              <span className="text-lg font-bold">visper</span>
              <span className="text-[#94A3B8]">│</span>
              <div className="flex items-center gap-1">
                <div className="h-2 w-2 rounded-full bg-[#3B82F6] animate-pulse shadow-[0_0_8px_rgba(59,130,246,0.6)]"></div>
                <span className="text-[#94A3B8] text-xs">live</span>
              </div>
            </div>
          </div>

          {/* Navigation Links */}
          <div className="flex items-center gap-1">
            {NAV_LINKS.map(({ path, label, symbol }) => (
              <Link
                key={path}
                to={path}
                className="group relative px-4 py-2 text-[#94A3B8] hover:text-[#F1F5F9] transition-all duration-200"
              >
                <div className="flex items-center gap-2">
                  <span className="text-[#3B82F6] opacity-0 group-hover:opacity-100 transition-opacity">
                    {symbol}
                  </span>
                  <span>{label}</span>
                </div>
                {/* Border hover effect */}
                <span className="absolute inset-0 border border-transparent group-hover:border-[#2D3748] rounded transition-all">
                  <span className="absolute top-0 left-0 text-[#3B82F6] text-xs opacity-0 group-hover:opacity-100 transition-opacity">
                    ╭
                  </span>
                  <span className="absolute top-0 right-0 text-[#3B82F6] text-xs opacity-0 group-hover:opacity-100 transition-opacity">
                    ╮
                  </span>
                  <span className="absolute bottom-0 left-0 text-[#3B82F6] text-xs opacity-0 group-hover:opacity-100 transition-opacity">
                    ╰
                  </span>
                  <span className="absolute bottom-0 right-0 text-[#3B82F6] text-xs opacity-0 group-hover:opacity-100 transition-opacity">
                    ╯
                  </span>
                </span>
              </Link>
            ))}
          </div>

          {/* To center the links */}
          <div />
        </div>
      </div>
    </nav>
  );
}
