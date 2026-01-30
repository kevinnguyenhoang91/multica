"use client"

import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarMenuAction,
} from "@multica/ui/components/ui/sidebar"
import { Button } from "@multica/ui/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { PlusSignIcon, Delete02Icon, Loading03Icon } from "@hugeicons/core-free-icons"
import { useHub } from "../hooks/use-hub"
import { useActiveAgent } from "../hooks/use-active-agent"

const STATUS_DOT: Record<string, string> = {
  connected: "bg-green-500/60",
  loading: "bg-yellow-500/50 animate-pulse",
  error: "bg-red-500/60",
  idle: "bg-muted-foreground/50",
}

const STATUS_LABEL: Record<string, string> = {
  connected: "Connected",
  loading: "Connecting...",
  error: "Disconnected",
  idle: "Idle",
}

export function HubSidebar() {
  const { status, hub, agents, fetchHub, createAgent, deleteAgent } = useHub()
  const activeAgentId = useActiveAgent((s) => s.activeAgentId)
  const setActiveAgentId = useActiveAgent((s) => s.setActiveAgentId)

  return (
    <>
      <SidebarGroup>
        <SidebarGroupLabel>Hub</SidebarGroupLabel>
        <SidebarGroupContent>
          <div className="flex items-center gap-2 px-2 py-1 text-sm">
            <span className={`size-2 rounded-full shrink-0 ${STATUS_DOT[status]}`} />
            <span className="text-muted-foreground/70 text-xs">{STATUS_LABEL[status]}</span>
          </div>
          {status === "connected" && hub && (
            <div className="px-2 text-xs text-muted-foreground/50 font-mono truncate">
              {hub.hubId}
            </div>
          )}
          {status === "error" && (
            <div className="px-2 pt-1">
              <Button variant="outline" size="sm" onClick={fetchHub} className="w-full text-xs">
                Retry
              </Button>
            </div>
          )}
        </SidebarGroupContent>
      </SidebarGroup>

      {status === "connected" && (
        <SidebarGroup>
          <SidebarGroupLabel>Agents</SidebarGroupLabel>
          <SidebarGroupAction onClick={createAgent} title="Create agent">
            <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} className="size-4" />
          </SidebarGroupAction>
          <SidebarGroupContent>
            <SidebarMenu>
              {agents.length === 0 && (
                <div className="px-2 py-2 text-xs text-muted-foreground/60">
                  No agents
                </div>
              )}
              {agents.map(agent => (
                <SidebarMenuItem key={agent.id}>
                  <SidebarMenuButton
                    isActive={agent.id === activeAgentId}
                    onClick={() => setActiveAgentId(agent.id)}
                    className="font-mono text-xs truncate"
                  >
                    {agent.id}
                  </SidebarMenuButton>
                  <SidebarMenuAction
                    showOnHover
                    onClick={() => {
                      if (activeAgentId === agent.id) setActiveAgentId(null)
                      deleteAgent(agent.id)
                    }}
                    title="Delete agent"
                  >
                    <HugeiconsIcon icon={Delete02Icon} strokeWidth={1.5} className="size-3.5 text-muted-foreground" />
                  </SidebarMenuAction>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      )}
    </>
  )
}
