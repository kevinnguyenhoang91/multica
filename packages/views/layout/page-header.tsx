"use client";

import { cn } from "@multica/ui/lib/utils";
import { SidebarTrigger, useOptionalSidebar } from "@multica/ui/components/ui/sidebar";

function MobileSidebarTrigger() {
  const sidebar = useOptionalSidebar();
  if (!sidebar) return null;

  const shouldShowTrigger = sidebar.isMobile || sidebar.state === "collapsed";
  if (!shouldShowTrigger) return null;

  return <SidebarTrigger className={cn("mr-2", sidebar.isMobile && "md:hidden")} />;
}

interface PageHeaderProps {
  children: React.ReactNode;
  className?: string;
}

export function PageHeader({ children, className }: PageHeaderProps) {
  return (
    <div className={cn("flex h-12 shrink-0 items-center border-b px-4", className)}>
      <MobileSidebarTrigger />
      {children}
    </div>
  );
}
