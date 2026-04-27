import { type ReactNode } from 'react'
import { PrimaryRail } from './PrimaryRail'
import { Sidebar } from './Sidebar'
import { ChannelHeader } from './ChannelHeader'
import { DisconnectBanner } from './DisconnectBanner'
import { StatusBar } from './StatusBar'
import { RuntimeStrip } from './RuntimeStrip'
import { AgentPanel } from '../agents/AgentPanel'
import { useAppStore } from '../../stores/app'

interface ShellProps {
  children: ReactNode
}

export function Shell({ children }: ShellProps) {
  const dmMode = useAppStore((s) => s.dmMode)

  return (
    <div className="office">
      <PrimaryRail />
      <Sidebar />
      <main className="main">
        <DisconnectBanner />
        {!dmMode && <ChannelHeader />}
        {!dmMode && <RuntimeStrip />}
        {children}
        <StatusBar />
      </main>
      <AgentPanel />
    </div>
  )
}
