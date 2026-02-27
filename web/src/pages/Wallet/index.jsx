import React, { useEffect, useState } from 'react'
import { Card, Typography, Button, Table, Tag } from '@douyinfe/semi-ui'
import { useNavigate } from 'react-router-dom'
import { useStore } from '../../store'
import { listTransactions } from '../../api/wallet'

const { Title, Text } = Typography

const TX_TYPE_MAP = {
  topup:            { label: '充值',     color: 'green' },
  subscription:     { label: '订阅扣款', color: 'orange' },
  product_purchase: { label: '购买',     color: 'orange' },
  refund:           { label: '退款',     color: 'light-blue' },
  bonus:            { label: '奖励',     color: 'teal' },
  referral_reward:  { label: '推荐奖励', color: 'teal' },
  redemption:       { label: '兑换码',   color: 'purple' },
  checkin_reward:   { label: '签到',     color: 'cyan' },
  admin_credit:     { label: '管理员入账', color: 'green' },
  admin_debit:      { label: '管理员扣款', color: 'red' },
}

const columns = [
  {
    title: '类型',
    dataIndex: 'type',
    render: (t) => {
      const info = TX_TYPE_MAP[t] ?? { label: t, color: 'grey' }
      return <Tag color={info.color} size="small">{info.label}</Tag>
    },
  },
  {
    title: '金额',
    dataIndex: 'amount',
    render: (v) => (
      <span style={{ color: v >= 0 ? '#52c41a' : '#f5222d', fontWeight: 600 }}>
        {v >= 0 ? '+' : ''}{v.toFixed(4)} CNY
      </span>
    ),
  },
  { title: '余额', dataIndex: 'balance_after', render: (v) => `${v.toFixed(4)} CNY` },
  { title: '描述', dataIndex: 'description' },
  {
    title: '时间',
    dataIndex: 'created_at',
    render: (v) => new Date(v).toLocaleString('zh-CN'),
  },
]

export default function WalletPage() {
  const navigate = useNavigate()
  const { wallet, account, subscriptions, refreshWallet } = useStore()
  const [txList, setTxList] = useState([])
  const [txTotal, setTxTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    refreshWallet()
  }, [])

  useEffect(() => {
    setLoading(true)
    listTransactions({ page, page_size: 20 })
      .then((res) => {
        setTxList(res.data.data ?? [])
        setTxTotal(res.data.total ?? 0)
      })
      .finally(() => setLoading(false))
  }, [page])

  return (
    <div>
      <Title heading={3} style={{ marginBottom: 24 }}>我的钱包</Title>

      <div style={{ display: 'flex', gap: 16, marginBottom: 24 }}>
        <Card style={{ flex: 1 }} shadows="always">
          <Text type="secondary">可用余额</Text>
          <div style={{ fontSize: 32, fontWeight: 700, color: '#1677ff', margin: '8px 0' }}>
            ¥ {wallet?.balance?.toFixed(2) ?? '0.00'}
          </div>
          <Button type="primary" onClick={() => navigate('/topup')}>立即充值</Button>
        </Card>
        <Card style={{ flex: 1 }} shadows="always">
          <Text type="secondary">历史累计充值</Text>
          <div style={{ fontSize: 24, fontWeight: 600, marginTop: 8 }}>
            ¥ {wallet?.lifetime_topup?.toFixed(2) ?? '0.00'}
          </div>
        </Card>
        <Card style={{ flex: 1 }} shadows="always">
          <Text type="secondary">活跃订阅</Text>
          <div style={{ fontSize: 24, fontWeight: 600, marginTop: 8 }}>
            {subscriptions.filter(s => s.status === 'active').length} 个
          </div>
        </Card>
      </div>

      <Card title="交易流水">
        <Table
          columns={columns}
          dataSource={txList}
          loading={loading}
          rowKey="id"
          pagination={{
            total: txTotal,
            currentPage: page,
            pageSize: 20,
            onPageChange: setPage,
          }}
        />
      </Card>
    </div>
  )
}
