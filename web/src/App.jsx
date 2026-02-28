import React, { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import WalletPage from './pages/Wallet'
import TopupPage from './pages/Topup'
import SubscriptionsPage from './pages/Subscriptions'
import RedeemPage from './pages/Redeem'
import AdminPage from './pages/Admin'
import CallbackPage from './pages/Callback'
import LoginPage from './pages/Login'
import { useStore } from './store'

export default function App() {
  const init = useStore((s) => s.init)

  useEffect(() => {
    init()
  }, [])

  return (
    <BrowserRouter>
      <Routes>
        {/* Auth pages — outside Layout, no auth required */}
        <Route path="/login" element={<LoginPage />} />
        <Route path="/callback" element={<CallbackPage />} />

        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/wallet" replace />} />
          <Route path="wallet" element={<WalletPage />} />
          <Route path="topup" element={<TopupPage />} />
          <Route path="subscriptions" element={<SubscriptionsPage />} />
          <Route path="redeem" element={<RedeemPage />} />
          <Route path="admin/*" element={<AdminPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
