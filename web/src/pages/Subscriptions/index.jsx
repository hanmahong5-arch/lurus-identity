import React, { useEffect, useState } from 'react'
import {
  Card, Typography, Button, Modal, Toast, Tag, Descriptions
} from '@douyinfe/semi-ui'
import { useStore } from '../../store'
import { listProducts, listPlans, checkout } from '../../api/subscription'
import { getTopupInfo } from '../../api/wallet'
import LurusBadge from '../../components/LurusBadge'

const { Title, Text } = Typography

const PLAN_CODE_LABEL = {
  free:       '免费版',
  basic:      'Basic',
  pro:        'Pro',
  enterprise: 'Enterprise',
}

function formatDate(d) {
  if (!d) return '永久'
  return new Date(d).toLocaleDateString('zh-CN')
}

function SubCard({ product, sub, onUpgrade }) {
  const planCode = sub?.plan_code ?? 'free'
  const expiresAt = sub?.expires_at
  const autoRenew = sub?.auto_renew

  return (
    <Card
      shadows="hover"
      style={{ minWidth: 240, flex: '1 1 240px' }}
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <LurusBadge productId={product.id} planCode={planCode} />
          <span>{product.name}</span>
        </div>
      }
      footer={
        <Button type="primary" size="small" onClick={() => onUpgrade(product)}>
          {planCode === 'free' ? '立即订阅' : '升级套餐'}
        </Button>
      }
    >
      <Descriptions
        size="small"
        data={[
          { key: '当前套餐', value: PLAN_CODE_LABEL[planCode] ?? planCode },
          { key: '到期日期', value: formatDate(expiresAt) },
          { key: '自动续费', value: autoRenew ? '开启' : '关闭' },
        ]}
      />
    </Card>
  )
}

export default function SubscriptionsPage() {
  const { subscriptions, wallet, refreshSubscriptions, refreshWallet } = useStore()
  const [products, setProducts] = useState([])
  const [upgradeProduct, setUpgradeProduct] = useState(null)
  const [plans, setPlans] = useState([])
  const [selectedPlan, setSelectedPlan] = useState(null)
  const [methods, setMethods] = useState([])
  const [method, setMethod] = useState('wallet')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    listProducts().then((res) => setProducts(res.data ?? []))
    getTopupInfo().then((res) => setMethods(res.data.payment_methods ?? []))
  }, [])

  async function openUpgrade(product) {
    setUpgradeProduct(product)
    const res = await listPlans(product.id)
    const pList = (res.data ?? []).filter((p) => p.status === 1 && p.code !== 'free')
    setPlans(pList)
    setSelectedPlan(pList[0] ?? null)
  }

  async function handleCheckout() {
    if (!selectedPlan) return
    setLoading(true)
    try {
      const res = await checkout({
        product_id: upgradeProduct.id,
        plan_id: selectedPlan.id,
        payment_method: method,
        return_url: window.location.origin + '/subscriptions',
      })
      if (res.data.pay_url) {
        window.open(res.data.pay_url, '_blank')
      }
      Toast.success('订阅成功')
      setUpgradeProduct(null)
      await Promise.all([refreshSubscriptions(), refreshWallet()])
    } catch (err) {
      Toast.error(err.response?.data?.error ?? '操作失败')
    } finally {
      setLoading(false)
    }
  }

  const subMap = Object.fromEntries(subscriptions.map((s) => [s.product_id, s]))

  return (
    <div>
      <Title heading={3} style={{ marginBottom: 24 }}>我的订阅</Title>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16 }}>
        {products.map((p) => (
          <SubCard
            key={p.id}
            product={p}
            sub={subMap[p.id]}
            onUpgrade={openUpgrade}
          />
        ))}
      </div>

      <Modal
        title={`订阅 ${upgradeProduct?.name}`}
        visible={!!upgradeProduct}
        onOk={handleCheckout}
        onCancel={() => setUpgradeProduct(null)}
        okText="确认购买"
        confirmLoading={loading}
        width={480}
      >
        <div style={{ marginBottom: 16 }}>
          <Text type="secondary">选择套餐</Text>
          <div style={{ display: 'flex', gap: 10, marginTop: 8, flexWrap: 'wrap' }}>
            {plans.map((p) => (
              <Card
                key={p.id}
                style={{
                  cursor: 'pointer',
                  flex: 1,
                  border: selectedPlan?.id === p.id ? '2px solid #1677ff' : '1px solid #e5e5e5',
                }}
                onClick={() => setSelectedPlan(p)}
                bodyStyle={{ padding: '12px 16px' }}
              >
                <div style={{ fontWeight: 600 }}>{PLAN_CODE_LABEL[p.code] ?? p.code}</div>
                <div style={{ fontSize: 20, color: '#1677ff', margin: '4px 0' }}>
                  ¥{p.price_cny}
                </div>
                <div style={{ fontSize: 12, color: '#8c8c8c' }}>{p.billing_cycle}</div>
              </Card>
            ))}
          </div>
        </div>

        <div>
          <Text type="secondary">支付方式</Text>
          <div style={{ display: 'flex', gap: 10, marginTop: 8, flexWrap: 'wrap' }}>
            <Button
              type={method === 'wallet' ? 'primary' : 'tertiary'}
              onClick={() => setMethod('wallet')}
            >
              钱包余额 (¥{wallet?.balance?.toFixed(2) ?? '0.00'})
            </Button>
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
        </div>
      </Modal>
    </div>
  )
}
