<div align="center">
  <a href="https://alistgo.com"><img width="100px" alt="logo" src="https://cdn.jsdelivr.net/gh/alist-org/logo@main/logo.svg"/></a>
  <p><em>🗂一个支持多存储的文件列表程序，使用 Gin 和 Solidjs。</em></p>
<div>
  <a href="https://goreportcard.com/report/github.com/alist-org/alist/v3">
    <img src="https://goreportcard.com/badge/github.com/alist-org/alist/v3" alt="latest version" />
  </a>
  <a href="https://github.com/alist-org/alist/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/Xhofe/alist" alt="License" />
  </a>
  <a href="https://github.com/alist-org/alist/actions?query=workflow%3ABuild">
    <img src="https://img.shields.io/github/actions/workflow/status/Xhofe/alist/build.yml?branch=main" alt="Build status" />
  </a>
  <a href="https://github.com/alist-org/alist/releases">
    <img src="https://img.shields.io/github/release/Xhofe/alist" alt="latest version" />
  </a>
  <a title="Crowdin" target="_blank" href="https://crwd.in/alist">
    <img src="https://badges.crowdin.net/alist/localized.svg">
  </a>
</div>
<div>
  <a href="https://github.com/alist-org/alist/discussions">
    <img src="https://img.shields.io/github/discussions/Xhofe/alist?color=%23ED8936" alt="discussions" />
  </a>
  <a href="https://discord.gg/F4ymsH4xv2">
    <img src="https://img.shields.io/discord/1018870125102895134?logo=discord" alt="discussions" />
  </a>
  <a href="https://github.com/alist-org/alist/releases">
    <img src="https://img.shields.io/github/downloads/Xhofe/alist/total?color=%239F7AEA&logo=github" alt="Downloads" />
  </a>
  <a href="https://hub.docker.com/r/xhofe/alist">
    <img src="https://img.shields.io/docker/pulls/xhofe/alist?color=%2348BB78&logo=docker&label=pulls" alt="Downloads" />
  </a>
  <a href="https://alistgo.com/zh/guide/sponsor.html">
    <img src="https://img.shields.io/badge/%24-sponsor-F87171.svg" alt="sponsor" />
  </a>
</div>
</div>

---

[English](./README.md) | 中文 | [日本語](./README_ja.md) | [Contributing](./CONTRIBUTING.md) | [CODE_OF_CONDUCT](./CODE_OF_CONDUCT.md)

## 功能

- [x] 多种存储
    - [x] 本地存储
    - [x] [阿里云盘](https://www.alipan.com/)
    - [x] OneDrive / Sharepoint（[国际版](https://www.office.com/), [世纪互联](https://portal.partner.microsoftonline.cn),de,us）
    - [x] [天翼云盘](https://cloud.189.cn) (个人云, 家庭云)
    - [x] [GoogleDrive](https://drive.google.com/)
    - [x] [123云盘](https://www.123pan.com/)
    - [x] FTP / SFTP
    - [x] [PikPak](https://www.mypikpak.com/)
    - [x] [S3](https://aws.amazon.com/cn/s3/)
    - [x] [Seafile](https://seafile.com/)
    - [x] [又拍云对象存储](https://www.upyun.com/products/file-storage)
    - [x] WebDav(支持无API的OneDrive/SharePoint)
    - [x] Teambition（[中国](https://www.teambition.com/ )，[国际](https://us.teambition.com/ )）
    - [x] [MediaFire](https://www.mediafire.com)
    - [x] [分秒帧](https://www.mediatrack.cn/)
    - [x] [ProtonDrive](https://proton.me/drive)
    - [x] [和彩云](https://yun.139.com/) (个人云, 家庭云，共享群组)
    - [x] [Yandex.Disk](https://disk.yandex.com/)
    - [x] [百度网盘](http://pan.baidu.com/)
    - [x] [UC网盘](https://drive.uc.cn)
    - [x] [夸克网盘](https://pan.quark.cn)
    - [x] [迅雷网盘](https://pan.xunlei.com)
    - [x] [蓝奏云](https://www.lanzou.com/)
    - [x] [蓝奏云优享版](https://www.ilanzou.com/)
    - [x] [阿里云盘分享](https://www.alipan.com/)
    - [x] [谷歌相册](https://photos.google.com/)
    - [x] [Mega.nz](https://mega.nz)
    - [x] [一刻相册](https://photo.baidu.com/)
    - [x] SMB
    - [x] [115](https://115.com/)
    - [X] Cloudreve
    - [x] [Dropbox](https://www.dropbox.com/)
    - [x] [飞机盘](https://www.feijipan.com/)
    - [x] [多吉云](https://www.dogecloud.com/product/oss)
- [x] 部署方便，开箱即用
- [x] 文件预览（PDF、markdown、代码、纯文本……）
- [x] 画廊模式下的图像预览
- [x] 视频和音频预览，支持歌词和字幕
- [x] Office 文档预览（docx、pptx、xlsx、...）
- [x] `README.md` 预览渲染
- [x] 文件永久链接复制和直接文件下载
- [x] 黑暗模式
- [x] 国际化
- [x] 受保护的路由（密码保护和身份验证）
- [x] WebDav (具体见 https://alistgo.com/zh/guide/webdav.html)
- [x] [Docker 部署](https://hub.docker.com/r/xhofe/alist)
- [x] Cloudflare workers 中转
- [x] 文件/文件夹打包下载
- [x] 网页上传(可以允许访客上传)，删除，新建文件夹，重命名，移动，复制
- [x] 离线下载
- [x] 跨存储复制文件
- [x] 单线程下载/串流的多线程下载加速

## 本 fork 的 115 Cloud 按需分页与请求保护

- 每个挂载内的 115 SDK HTTP 请求共用限速器，包括目录的每一页，以及并发打开不同目录的请求。默认 `limit_rate=0.5`（平均每 2 秒一次，突发额度为 1）。**旧配置中的 `0` 或负值也使用 0.5，不再代表无限速**；已有正值保持不变，例如 `0.2` 表示平均每 5 秒一次。这不是 115 官方承诺的安全频率。
- HTTP 429、HTTP 405（按风控拦截保守处理），以及 JSON 失败响应中的 `errno` / `errNo` / `code` / 解密后上传响应的 `statuscode` 为 911 或 429 时，立即停止当前请求，不自动重试。该挂载进入至少 5 分钟冷却；如果 `Retry-After` 指定更长时间，则使用更长时间。冷却期内新请求直接返回包含恢复时间的错误。
- 分页请求支持取消和超时，单个 HTTP 请求（含等待限速额度）最多 30 秒；低速率或长队列可能在请求发送前就超时，此时直接失败而不是继续请求。其他 SDK 操作的中途取消能力未在本次补丁中改造。遇到异常空页或偏移不匹配时停止，避免持续请求。列表中途失败不会把已获取的部分条目作为完整目录返回。
- **网页浏览改为真正按需分页**：请求第 N 页只读取 `offset=(N-1)*per_page` 对应的一个上游范围，不预取后续页，不为凑满过滤后的页面继续扫描。`per_page<=0`（包括旧前端的“显示全部”）在 115 Cloud 上按 50 条处理，最大 500 条。`page_size` 只用于仍需完整列表的旧接口，不再决定网页分页大小。
- 配套前端将 115 的“全部 / 自动加载”切换成手动上一页、下一页，尊重后端返回的实际页大小；排序在 115 上游执行并回到第一页。必须同时部署配套前端，旧版前端的“全部”模式可能只显示首批条目且不显示翻页按钮。
- 子目录使用路径定位，不扫描完整父目录；115 的文件详情不再额外拉同级完整列表，因此相关文件/字幕自动发现不再覆盖整个目录。打开冷链接可能额外请求路径、挂载根信息，不能把“一页列表一次请求”理解为整个导航总共只有一个 HTTP 请求。
- 独立的按页缓存按挂载实例、目录、偏移、页大小和排序隔离，不污染完整列表缓存，不把部分页交给完整目录索引更新钩子；刷新和本地写操作使旧页失效。
- 隐藏和权限过滤只作用于当前上游页，允许空页/短页；`has_more` 基于上游范围，前端不以可见条目数量判断结束。不扫描全目录计算精确可见总数：`total_exact=false`，配套前端不显示精确总页数。
- WebDAV、目录选择器 `/fs/dirs`、索引、别名/加密等包装驱动及显式完整列表操作仍可能全量读取，但沿用逐请求限速。分页尚不支持目录内叠加其他虚拟挂载，也不支持“文件夹置底”；遇到这些情况明确报错，不偷偷退回全量扫描。
- 限速和冷却仅在当前进程、当前挂载内共享；同账号的其他挂载、其他 AList 实例及 115 客户端不共享额度。修改配置重新初始化会重建限速器；进程重启会丢失内存冷却状态，**不要靠反复重启试探风控**。这个补丁不能保证不触发风控，也不能解除已有风控。
- 上游分页仅对 `115 Cloud` 启用；通用列表层新增可选接口，未实现此接口的驱动保留原行为。不改变 `115 Open`、分享驱动和实际文件数据传输的限速。建议暂停自动扫描及频繁强制刷新，利用已有目录缓存。

### 获取配套前端与验证

前端修改以固定版本补丁保存在 [`web-patches/115-on-demand.patch`](web-patches/115-on-demand.patch)，无需再 fork 前端仓库。详见 [部署与 API 说明](web-patches/README.md)。

**不要在小内存业务服务器上构建。** 配套脚本默认禁止本地执行，并拒绝可用 RAM 不足 3 GiB 或容器内存限制低于 4 GiB 的环境。`.github/workflows/115-on-demand.yml` 在面向 `main` 的 PR 创建、更新时自动验证，并保留手动触发；测试和前后端构建在 GitHub 托管机器上串行执行，不发布、不部署。本轮按要求没有运行构建、测试编译或安装依赖，不能把当前源码视作已经通过最终编译验证的发布版。

新增测试使用模拟 HTTP/本地临时数据库，不使用网盘凭据、不请求真实 115。先在 CI 验证，再备份并部署；不要直接替换线上程序。

## 文档

<https://alistgo.com/zh/>

## API 文档（通过 Apifox 提供）

<https://alist-public.apifox.cn/>

## Demo

<https://al.nn.ci>

## 讨论

一般问题请到[讨论论坛](https://github.com/alist-org/alist/discussions) ，**issue仅针对错误报告和功能请求。**

## 赞助

AList 是一个开源软件，如果你碰巧喜欢这个项目，并希望我继续下去，请考虑赞助我或提供一个单一的捐款！感谢所有的爱和支持：https://alistgo.com/zh/guide/sponsor.html

### 特别赞助

- [VidHub](https://apps.apple.com/app/apple-store/id1659622164?pt=118612019&ct=alist&mt=8) - 苹果生态下优雅的网盘视频播放器，iPhone，iPad，Mac，Apple TV全平台支持。

## 贡献者

Thanks goes to these wonderful people:

[![Contributors](http://contrib.nn.ci/api?repo=alist-org/alist&repo=alist-org/alist-web&repo=alist-org/docs)](https://github.com/alist-org/alist/graphs/contributors)

## 许可

`AList` 是在 AGPL-3.0 许可下许可的开源软件。

## 免责声明
- 本程序为免费开源项目，旨在分享网盘文件，方便下载以及学习golang，使用时请遵守相关法律法规，请勿滥用；
- 本程序通过调用官方sdk/接口实现，无破坏官方接口行为；
- 本程序仅做302重定向/流量转发，不拦截、存储、篡改任何用户数据；
- 在使用本程序之前，你应了解并承担相应的风险，包括但不限于账号被ban，下载限速等，与本程序无关；
- 如有侵权，请通过[邮件](mailto:i@nn.ci)与我联系，会及时处理。

---

> [@博客](https://nn.ci/) · [@GitHub](https://github.com/alist-org) · [@Telegram群](https://t.me/alist_chat) · [@Discord](https://discord.gg/F4ymsH4xv2)
