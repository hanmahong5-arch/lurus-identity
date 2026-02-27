import React, { useState, useEffect } from 'react'
import {
  Card, Typography, Table, Input, Button, Modal, InputNumber, Toast, Tag
} from '@douyinfe/semi-ui'
import axios from 'axios'

const { Title } = Typography

const adminClient = axios.create({ baseURL: '/admin/v1', timeout: 15000 })
adminClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

export default function AdminPage() {
  const [accounts, setAccounts] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(false)
  const [adjustTarget, setAdjustTarget] = useState(null)
  const [adjustAmount, setAdjustAmount] = useState(0)
  const [adjustDesc, setAdjustDesc] = useState('')
  const [adjustLoading, setAdjustLoading] = useState(false)

  async function fetchAccounts(p = page, kw = keyword) {
    setLoading(true)
    try {
      const res = await adminClient.get('/accounts', { params: { page: p, page_size: 20, keyword: kw } })
      setAccounts(res.data.accounts ?? res.data.data ?? [])
      setTotal(res.data.total ?? 0)
    } catch (err) {
      Toast.error('加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchAccounts() }, [page])

  async function handleAdjust() {
    if (!adjustTarget || !adjustDesc) return
    setAdjustLoading(true)
    try {
      await adminClient.post(`/accounts/${adjustTarget.id}/wallet/adjust`, {
        amount: adjustAmount,
        description: adjustDesc,
      })
      Toast.success('余额调整成功')
      setAdjustTarget(null)
      setAdjustAmount(0)
      setAdjustDesc('')
      fetchAccounts()
    } catch (err) {
      Toast.error(err.response?.data?.error ?? '操作失败')
    } finally {
      setAdjustLoading(false)
    }
  }

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: 'Lurus ID', dataIndex: 'lurus_id' },
    { title: '昵称', dataIndex: 'nickname' },
    { title: '邮箱', dataIndex: 'email' },
    {
      title: 'VIP',
      dataIndex: 'vip_level',
      render: (v) => v > 0 ? <Tag color="yellow">Lv.{v}</Tag> : <Tag>普通</Tag>,
    },
    {
      title: '操作',
      render: (_, row) => (
        <Button size="small" onClick={() => setAdjustTarget(row)}>余额调整</Button>
      ),
    },
  ]

  return (
    <div>
      <Title heading={3} style={{ marginBottom: 24 }}>管理后台 — 账号列表</Title>

      <div style={{ display: 'flex', gap: 10, marginBottom: 16 }}>
        <Input
          value={keyword}
          onChange={setKeyword}
          placeholder="搜索邮箱 / Lurus ID"
          style={{ width: 280 }}
          onEnterPress={() => { setPage(1); fetchAccounts(1, keyword) }}
        />
        <Button onClick={() => { setPage(1); fetchAccounts(1, keyword) }}>搜索</Button>
      </div>

      <Card>
        <Table
          columns={columns}
          dataSource={accounts}
          loading={loading}
          rowKey="id"
          pagination={{
            total,
            currentPage: page,
            pageSize: 20,
            onPageChange: (p) => setPage(p),
          }}
        />
      </Card>

      <Modal
        title={`余额调整 — ${adjustTarget?.lurus_id ?? ''}`}
        visible={!!adjustTarget}
        onOk={handleAdjust}
        onCancel={() => setAdjustTarget(null)}
        okText="确认"
        confirmLoading={adjustLoading}
      >
        <div style={{ marginBottom: 12 }}>
          <div>金额（正数=入账，负数=扣款）</div>
          <InputNumber
            value={adjustAmount}
            onChange={setAdjustAmount}
            style={{ width: '100%', marginTop: 4 }}
          />
        </div>
        <div>
          <div>备注说明</div>
          <Input
            value={adjustDesc}
            onChange={setAdjustDesc}
            style={{ marginTop: 4 }}
            placeholder="请填写调整原因"
          />
        </div>
      </Modal>
    </div>
  )
}
