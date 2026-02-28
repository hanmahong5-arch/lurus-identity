import React from 'react'
import { Button, Card, Typography, Divider } from '@douyinfe/semi-ui'
import { login } from '../../auth'

const { Title, Text } = Typography

export default function LoginPage() {
  function handleEmailLogin() {
    // Preserve the intended destination before redirecting to Zitadel.
    const returnTo = sessionStorage.getItem('login_return') || '/wallet'
    sessionStorage.setItem('login_return', returnTo)
    login()
  }

  function handleWechatLogin() {
    // The backend handles the WeChat OAuth redirect dance.
    // After WeChat auth, the server redirects to /callback?lurus_token=<token>.
    const returnTo = window.location.pathname !== '/login' ? window.location.pathname : '/wallet'
    sessionStorage.setItem('login_return', returnTo)
    window.location.href = '/api/v1/auth/wechat'
  }

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '100vh',
        background: '#f5f5f5',
      }}
    >
      <Card style={{ width: 360, padding: '32px 24px' }} shadows="always">
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title heading={3} style={{ color: '#1677ff', marginBottom: 4 }}>Lurus</Title>
          <Text type="secondary">登录你的账号</Text>
        </div>

        <Button
          type="primary"
          size="large"
          block
          onClick={handleEmailLogin}
          style={{ marginBottom: 16 }}
        >
          邮箱 / 密码登录
        </Button>

        <Divider>或</Divider>

        <Button
          size="large"
          block
          onClick={handleWechatLogin}
          style={{ marginTop: 16 }}
          icon={
            <svg width="18" height="18" viewBox="0 0 24 24" fill="#07c160" style={{ verticalAlign: 'middle', marginRight: 6 }}>
              <path d="M8.5 11a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3zm7 0a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3zM12 2C6.477 2 2 6.253 2 11.5c0 2.97 1.44 5.61 3.7 7.37L5 22l3.05-1.54A10.16 10.16 0 0 0 12 21c5.523 0 10-4.253 10-9.5S17.523 2 12 2z"/>
            </svg>
          }
        >
          微信扫码登录
        </Button>
      </Card>
    </div>
  )
}
