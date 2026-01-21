import { Navbar } from "@/components/navbar";
import HomeView from "@/features/home/views/home-view";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/")({ component: App });

function App() {
  return (
    <div className="min-h-screen flex flex-col bg-[#0A0E14]">
      <Navbar />
      <HomeView />
    </div>
  );
}
