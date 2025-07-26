# Helius 节点延迟比较工具

本项目用于比较 Helius 两种节点类型的平均区块延迟时间和平均网络延迟时间，帮助开发者分析不同类型节点在生产环境中的响应性能差异。

## ✨ 特性

- 自动计算平均区块延迟时间和平均网络延迟时间
- 日志自动记录至 `metrics.log`
- 支持将日志可视化为 `metrics.png`，便于直观展示性能差异

## 📊 延迟时间说明

平均延迟计算方式如下：

平均延迟时间 = sum(LocalTime - BlockTime) / n


- `LocalTime`：本地获取到区块数据的时间，**包括 grpc proto block 转换所需的时间**
- `BlockTime`：区块本身的链上时间戳


## 运行

1. 运行主程序
```bash
git clone https://github.com/liyouyoumao/helius-latency-analyzer
```

```bash
cd helius-latency-analyzer && go run .
```

2. 可视化结果

```bash
go run draw/main.go
```

