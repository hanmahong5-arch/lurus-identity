import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Spin, Typography } from '@douyinfe/semi-ui'
import { handleCallback } from '../../auth'

const { Text } = Typography

export default function CallbackPage() {
  const navigate = useNavigate()
  const [error, setError] = useState(null)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const code   = params.get('code')
    const err    = params.get('error')
    const errDesc = params.get('error_description')

    if (err) {
      setError(`${err}: ${errDesc || ''}`)
      return
    }

    if (!code) {
      setError('No authorization code received.')
      return
    }

    handleCallback(code)
      .then(() => {
        const returnTo = sessionStorage.getItem('login_return') || '/wallet'
        sessionStorage.removeItem('login_return')
        navigate(returnTo, { replace: true })
      })
      .catch(e => setError(e.message))
  }, [])

  if (error) {
    return (
      <div style={{ textAlign: 'center', marginTop: 80 }}>
        <div style={{ fontSize: 40 }}>❌</div>
        <div style={{ marginTop: 16, color: '#f5222d' }}>登录失败</div>
        <Text type="tertiary" style={{ fontSize: 13 }}>{error}</Text>
        <div style={{ marginTop: 16 }}>
          <a href="/">重新登录</a>
        </div>
      </div>
    )
  }

  return (
    <div style={{ textAlign: 'center', marginTop: 80 }}>
      <Spin size="large" />
      <div style={{ marginTop: 16, color: '#8c8c8c' }}>正在完成登录...</div>
    </div>
  )
}
