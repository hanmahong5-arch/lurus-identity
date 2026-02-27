import React, { useEffect, useState } from 'react'
import {
  Card, Typography, Button, InputNumber, Modal, Toast, Spin, Tag
} from '@douyinfe/semi-ui'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { getTopupInfo, createTopup, getOrder } from '../../api/wallet'
import { useStore } from '../../store'

const { Title, Text } = Typography

const QUICK_AMOUNTS = [10, 30, 100, 300, 500]

export default function TopupPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refreshWallet } = useStore()

  const [methods, setMethods] = useState([])
  const [amount, setAmount] = useState(100)
  const [customAmount, setCustomAmount] = useState('')
  const [method, setMethod] = useState('')
  const [confirmVisible, setConfirmVisible] = useState(false)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    getTopupInfo().then((res) => {
      const list = res.data.payment_methods ?? []
      setMethods(list)
      if (list.length > 0) setMethod(list[0].id)
    })
  }, [])

  // Result polling (when returning from payment)
  const orderNo = searchParams.get('order_no')
  const [pollResult, setPollResult] = useState(null)
  const [polling, setPolling] = useState(false)

  useEffect(() => {
    if (!orderNo) return
    setPolling(true)
    const start = Date.now()
    const MAX_MS = 5 * 60 * 1000

    const timer = setInterval(async () => {
      if (Date.now() - start > MAX_MS) {
        clearInterval(timer)
        setPolling(false)
        setPollResult('timeout')
        return
      }
      try {
        const res = await getOrder(orderNo)
        if (res.data.status === 'paid') {
          clearInterval(timer)
          setPolling(false)
          setPollResult('success')
          refreshWallet()
        } else if (res.data.status === 'failed' || res.data.status === 'cancelled') {
          clearInterval(timer)
          setPolling(false)
          setPollResult('failed')
        }
      } catch (_) {}
    }, 3000)

    return () => clearInterval(timer)
  }, [orderNo])

  const actualAmount = customAmount ? parseFloat(customAmount) : amount

  async function handleConfirm() {
    setLoading(true)
    try {
      const res = await createTopup({
        amount_cny: actualAmount,
        payment_method: method,
        return_url: window.location.origin + '/topup',
      })
      const { pay_url, order_no } = res.data
      if (pay_url) window.open(pay_url, '_blank')
      navigate(`/topup?order_no=${order_no}`)
    } catch (err) {
      Toast.error(err.response?.data?.error ?? '创建订单失败')
    } finally {
      setLoading(false)
      setConfirmVisible(false)
    }
  }

  if (polling) {
    return (
      <div style={{ textAlign: 'center', marginTop: 80 }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>正在等待支付结果...</div>
      </div>
    )
  }

  if (pollResult === 'success') {
    return (
      <div style={{ textAlign: 'center', marginTop: 80 }}>
        <div style={{ fontSize: 48 }}>✅</div>
        <Title heading={4} style={{ marginTop: 16 }}>充值成功！</Title>
        <Button type="primary" onClick={() => navigate('/wallet')} style={{ marginTop: 16 }}>
          查看钱包
        </Button>
      </div>
    )
  }

  if (pollResult === 'failed') {
    return (
      <div style={{ textAlign: 'center', marginTop: 80 }}>
        <div style={{ fontSize: 48 }}>❌</div>
        <Title heading={4} style={{ marginTop: 16 }}>支付未完成</Title>
        <Button onClick={() => navigate('/topup')} style={{ marginTop: 16 }}>重新充值</Button>
      </div>
    )
  }

  return (
    <div style={{ maxWidth: 600 }}>
      <Title heading={3} style={{ marginBottom: 24 }}>钱包充值</Title>

      <Card title="选择金额" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10, marginBottom: 16 }}>
          {QUICK_AMOUNTS.map((v) => (
            <Button
              key={v}
              type={amount === v && !customAmount ? 'primary' : 'tertiary'}
              onClick={() => { setAmount(v); setCustomAmount('') }}
            >
              ¥{v}
            </Button>
          ))}
        </div>
        <InputNumber
          placeholder="自定义金额"
          value={customAmount}
          onChange={(v) => setCustomAmount(v)}
          min={1}
          max={50000}
          prefix="¥"
          style={{ width: 200 }}
        />
      </Card>

      <Card title="支付方式" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
          {methods.map((m) => (
            <Button
              key={m.id}
              type={method === m.id ? 'primary' : 'tertiary'}
              onClick={() => setMethod(m.id)}
            >
              {m.name}
            </Button>
          ))}
        </div>
      </Card>

      <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
        <Button
          type="primary"
          size="large"
          disabled={!method || !actualAmount || actualAmount <= 0}
          onClick={() => setConfirmVisible(true)}
        >
          确认充值 ¥{actualAmount}
        </Button>
      </div>

      <Modal
        title="确认充值"
        visible={confirmVisible}
        onOk={handleConfirm}
        onCancel={() => setConfirmVisible(false)}
        okText="确认支付"
        confirmLoading={loading}
      >
        <p>充值金额：<strong>¥{actualAmount}</strong></p>
        <p>支付方式：<strong>{methods.find(m => m.id === method)?.name}</strong></p>
        <p>点击确认后将打开支付页面，请在新标签页完成支付。</p>
      </Modal>
    </div>
  )
}
