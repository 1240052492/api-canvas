# API Canvas Integration

New API + TapCanvas 轻量级集成方案，实现 SSO 单点登录、共享钱包、AI 模型网关。

## 项目结构

- `apps/new-api/` - AI 模型网关（Go）
- `apps/hono-api/` - TapCanvas API服务（TypeScript/Hono）
- `apps/web/` - TapCanvas Web前端（TypeScript/React）
- `docs/` - 设计文档和实施计划
- `tests/` - 集成测试

## 快速开始

详见 [设计文档](docs/superpowers/specs/2026-10-03-new-api-tapcanvas-integration-design.md) 和 [实施计划](docs/superpowers/plans/2026-10-03-new-api-tapcanvas-integration.md)。

## 技术栈

- **New API**: Go 1.19+, Gin, GORM, PostgreSQL/MySQL/SQLite
- **TapCanvas**: Node.js 18+, TypeScript, Hono, Prisma, PostgreSQL

## License

MIT
