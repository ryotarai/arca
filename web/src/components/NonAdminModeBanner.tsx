import { setAdminViewMode } from "@/lib/api";

export function NonAdminModeBanner() {
  const handleBackToAdmin = async () => {
    await setAdminViewMode("admin");
    window.location.reload();
  };

  return (
    <div className="fixed top-0 left-0 right-0 z-50 flex items-center justify-center gap-4 bg-zinc-800/90 backdrop-blur-sm border-b border-zinc-700 px-4 py-1.5 text-sm text-zinc-300">
      <span>You are in non-admin mode</span>
      <button
        onClick={handleBackToAdmin}
        className="rounded px-2.5 py-0.5 text-xs font-medium bg-zinc-700 hover:bg-zinc-600 text-zinc-200 transition-colors border border-zinc-600"
      >
        Back to admin
      </button>
    </div>
  );
}
