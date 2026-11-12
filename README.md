# vertex-qui

基于 [autobrr/qui](https://github.com/autobrr/qui) 修改，在 qui 的 qBittorrent 多实例管理上加了刷流模块：
从站点 RSS 抢新种，放进最合适的下载器，空间不够时删掉产出最低的种子。

- 刷流使用说明：[BRUSH.md](BRUSH.md)
- qui 本身的功能说明：[getqui.com](https://getqui.com)

## 部署

```bash
docker run -d \
  -p 7476:7476 \
  -v $(pwd)/config:/config \
  yuukizekk/vertex-qui:latest
```

打开 http://localhost:7476 ，按 [BRUSH.md](BRUSH.md) 的「快速上手」配置。

## 许可

与 qui 相同，GPL-2.0-or-later，见 [LICENSE](LICENSE)。
