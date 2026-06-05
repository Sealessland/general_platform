# 订单状态机

## 状态定义

- `CREATED`：已下单，待支付
- `PAID`：已支付
- `SHIPPED`：已发货
- `FINISHED`：已完成
- `CANCELLED`：已取消
- `REFUNDING`：退款中
- `REFUNDED`：已退款

## 合法流转

| 当前状态 | 目标状态 | 触发条件 |
|---|---|---|
| CREATED | PAID | 模拟支付成功 |
| CREATED | CANCELLED | 用户取消或支付超时 |
| PAID | SHIPPED | 商家发货 |
| SHIPPED | FINISHED | 用户确认收货或系统自动完成 |
| PAID | REFUNDING | 发货前发起退款 |
| SHIPPED | REFUNDING | 发货后发起退款 |
| REFUNDING | REFUNDED | 商家审批通过并退款完成 |

## 规则

- 终态为 `FINISHED`、`CANCELLED`、`REFUNDED`
- 对已支付、已发货或终态订单重复支付，应在应用层拒绝或按幂等处理
- 订单取消和退款完成会触发库存释放或库存恢复
- 每一次合法状态变化都必须写入 `order_events`
