import { ArrowRightIcon, CheckIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyCursor } from "./empty-cursor";

export function TerminalContent() {
  return (
    <div className="p-6 md:p-8 space-y-4 text-sm md:text-base">
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">1</span>
        <div className="flex-1">
          <span className="text-[#64748B]">
            # connect with strangers around the world
          </span>
        </div>
      </div>

      {/* Line 2: Empty */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">2</span>
        <div className="flex-1"></div>
      </div>

      {/* Line 3: Command */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">3</span>
        <div className="flex-1">
          <span className="text-[#3B82F6]">ssh</span>{" "}
          <span className="text-[#F1F5F9]">visper.chat</span>
        </div>
      </div>

      {/* Line 4: Empty */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">4</span>
        <div className="flex-1"></div>
      </div>

      {/* Line 5: Features */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">5</span>
        <div className="flex-1 space-y-2">
          <div className="flex items-center gap-2">
            <CheckIcon className="text-[#10B981]" />

            <span className="text-[#94A3B8]">anonymous messaging</span>
          </div>
        </div>
      </div>

      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">6</span>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <CheckIcon className="text-[#10B981]" />
            <span className="text-[#94A3B8]">ephemeral conversations</span>
          </div>
        </div>
      </div>

      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">7</span>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <CheckIcon className="text-[#10B981]" />
            <span className="text-[#94A3B8]">real-time WebSocket</span>
          </div>
        </div>
      </div>

      {/* Line 8: Empty */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">8</span>
        <div className="flex-1"></div>
      </div>

      {/* Line 9: CTA */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">9</span>
        <div className="flex-1">
          <span className="text-[#64748B]">
            # start chatting, no registration required
          </span>
        </div>
      </div>

      {/* Line 10: Empty */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">10</span>
        <div className="flex-1"></div>
      </div>

      {/* Line 11: Button */}
      <div className="flex gap-4">
        <span className="text-[#2D3748] select-none">11</span>

        <Button className="w-auto p-5" variant="blue">
          <span>join now</span>
          <ArrowRightIcon />
        </Button>
      </div>

      <EmptyCursor />
    </div>
  );
}
