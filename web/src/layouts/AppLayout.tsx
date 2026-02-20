import { Outlet } from 'react-router-dom'
import { useState } from 'react'
import { Sidebar } from '@/components/Sidebar'
import { Navbar } from '@/components/Navbar'

export function AppLayout() {
  const [sidebarCompact, setSidebarCompact] = useState(false)

  return (
    <div className="min-h-screen bg-[#0c1222]">
      {/* Sidebar */}
      <Sidebar compact={sidebarCompact} setCompact={setSidebarCompact} />

      {/* Main content */}
      <div className={`transition-all duration-300 ${sidebarCompact ? 'lg:pl-20' : 'lg:pl-64'}`}>
        {/* Navbar */}
        <Navbar sidebarCompact={sidebarCompact} />

        {/* Page content */}
        <main className="pt-16 min-h-screen">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
