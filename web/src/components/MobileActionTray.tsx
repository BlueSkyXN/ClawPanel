import { useEffect, useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';

interface Props {
  label: string;
  children: ReactNode;
}

export default function MobileActionTray({ label, children }: Props) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const sync = () => {
      if (window.innerWidth >= 640) setOpen(false);
    };
    sync();
    window.addEventListener('resize', sync);
    return () => window.removeEventListener('resize', sync);
  }, []);

  return (
    <>
      <div className="hidden items-center gap-2 sm:flex">
        {children}
      </div>
      <div className="sm:hidden w-full">
        <button
          onClick={() => setOpen(v => !v)}
          className="page-modern-action w-full justify-between px-3.5 py-2.5 text-xs font-medium"
        >
          <span>{label}</span>
          <ChevronDown size={14} className={`transition-transform ${open ? 'rotate-180' : ''}`} />
        </button>
        {open && (
          <div className="mt-2 grid grid-cols-1 gap-2 rounded-2xl border border-blue-100/70 bg-white/50 p-2 backdrop-blur-xl dark:border-blue-400/15 dark:bg-slate-950/28 [&>*]:w-full [&>*]:justify-center">
            {children}
          </div>
        )}
      </div>
    </>
  );
}
