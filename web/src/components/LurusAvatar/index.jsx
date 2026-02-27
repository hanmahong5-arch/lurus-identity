import React from 'react'
import { Popover, Avatar, Tag } from '@douyinfe/semi-ui'
import LurusBadge from '../LurusBadge'
import './avatar.css'

const VIP_COLORS = ['#8c8c8c', '#1890ff', '#52c41a', '#faad14', '#f5222d']

/**
 * LurusAvatar shows a user avatar with Lurus ID.
 * On hover, displays a popover with VIP level and active diamond badges.
 *
 * @param {{ account: object, subscriptions: Array, size?: number }} props
 */
export default function LurusAvatar({ account, subscriptions = [], size = 40 }) {
  if (!account) return null

  const lurusID = account.lurus_id ?? '—'
  const vipLevel = account.vip_level ?? 0
  const vipColor = VIP_COLORS[Math.min(vipLevel, VIP_COLORS.length - 1)]
  const initial = (account.nickname || account.email || 'L')[0].toUpperCase()

  // Only show non-free subscriptions
  const activeSubs = subscriptions.filter(
    (s) => s.status === 'active' || s.status === 'grace' || s.status === 'trial'
  )

  const content = (
    <div className="lurus-popover">
      <div className="popover-header">
        <span className="popover-lurus-id">{lurusID}</span>
        {vipLevel > 0 && (
          <Tag color="yellow" size="small" style={{ marginLeft: 8 }}>
            ★ VIP Lv.{vipLevel}
          </Tag>
        )}
      </div>
      {activeSubs.length > 0 ? (
        <div className="popover-badges">
          {activeSubs.map((sub) => (
            <LurusBadge
              key={sub.product_id}
              productId={sub.product_id}
              planCode={sub.plan_code}
              size="sm"
            />
          ))}
        </div>
      ) : (
        <div className="popover-no-sub">暂无订阅</div>
      )}
    </div>
  )

  return (
    <Popover content={content} position="bottomRight" showArrow>
      <span className="lurus-avatar-wrap">
        {account.avatar_url ? (
          <Avatar
            size={size}
            src={account.avatar_url}
            style={{ cursor: 'pointer' }}
          />
        ) : (
          <Avatar
            size={size}
            style={{ background: vipColor, cursor: 'pointer', fontSize: size * 0.4 }}
          >
            {initial}
          </Avatar>
        )}
        {vipLevel > 0 && (
          <span
            className="vip-dot"
            style={{ background: vipColor }}
            title={`VIP Lv.${vipLevel}`}
          />
        )}
      </span>
    </Popover>
  )
}
