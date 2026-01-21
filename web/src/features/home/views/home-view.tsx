import { TerminalContent } from "../components/terminal-content";
import { TerminalFooter } from "../components/terminal-footer";
import { TerminalHeader } from "../components/terminal-header";

export default function HomeView() {
  return (
    <div className="bg-[#0A0E14] text-[#94A3B8] font-mono">
      <div className="container mx-auto px-4 py-8 md:py-16">
        <div className="max-w-4xl mx-auto">
          <div className="border border-[#2D3748] rounded-t-lg bg-[#0A0E14] overflow-hidden">
            <TerminalHeader />
            <TerminalContent />
          </div>

          <TerminalFooter />
        </div>
      </div>
    </div>
  );
}
