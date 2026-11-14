# PEEK LOFTER

无需登录、开箱即用的 LOFTER 爬虫。

如果你是 AI 助手或开发者，建议先阅读 [AGENTS.md](AGENTS.md)，其中包含架构和实现细节。

## ✨ 特性

- **标签**：抓取指定标签下的作品，按时间切片以绕过访问限制。
- **用户**：查看用户资料、作品、喜欢和推荐。
- **作品**：保存正文、Markdown、图片和作品信息，支持单篇或批量。
- **简单**：不用登录，不用配置 Cookie，下载后直接运行。

## 🚀 安装

需要 Go 1.26 或更新版本。

~~~bash
git clone https://github.com/fltLi/peek-lofter.git
cd peek-lofter
go build -trimpath -o peek-lofter ./cmd/peek-lofter
~~~

也可以：

~~~bash
go install github.com/fltLi/peek-lofter/cmd/peek-lofter@latest
~~~

可选短别名 `plft`：把 `peek-lofter` 复制或软链为 `plft` 即可。

## 📖 使用

> [!TIP]
> 完整参数用 `--help` 查看；输出结构和本地浏览、合并等细节见 [docs/cli.md](docs/cli.md)。

~~~bash
# 标签抓取
peek-lofter tag example-tag --since 2024-01-01 --out ./data/example-tag
peek-lofter post https://example-user.lofter.com/post/abc_def --out ./dl --with-media  # 单篇下载

# 批量下载
peek-lofter post --from refs.jsonl --out ./dl --with-media
peek-lofter tag example-tag --since 2024-01-01 | peek-lofter post --out ./dl  # 标签直接下载
peek-lofter export ./dl --format txt --layout author-title --out ./text  # 导出
~~~

## 📄 许可证

Code: [MIT](LICENSE), 2026, fltLi

本项目由 DeepSeek-V4.1-Flash 参与实现。
