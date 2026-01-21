export function TerminalFooter() {
  return (
    <div className="border-x border-b border-[#2D3748] rounded-b-lg bg-[#0A0E14] px-6 py-3">
      <div className="flex items-center justify-between text-xs md:text-sm">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <div className="w-2 h-2 rounded-full bg-[#10B981] animate-pulse"></div>
            <span className="text-[#94A3B8]">1,247 users online</span>
          </div>
          <span className="text-[#2D3748]">│</span>
          <div className="hidden sm:flex items-center gap-2">
            <span className="text-[#94A3B8]">89 active rooms</span>
          </div>
        </div>
        <div className="text-[#64748B]">v1.0.0</div>
      </div>
    </div>
  );
}
