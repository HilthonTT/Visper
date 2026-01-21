export function TerminalHeader() {
  return (
    <div className="flex items-center gap-2 px-4 py-3 bg-[#0A0E14] border-b border-[#2D3748]">
      <div className="flex gap-1.5">
        <div className="size-3 rounded-full bg-[#EF4444]" />
        <div className="size-3 rounded-full bg-[#F59E0B]" />
        <div className="size-3 rounded-full bg-[#10B981]" />
      </div>
      <div className="flex-1 text-center text-sm text-[#94A3B8]">visper.sh</div>
    </div>
  );
}
