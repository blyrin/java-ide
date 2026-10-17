# java-ide

一个用 [MyGo](https://github.com/egoist/mygo) 原生 UI 实现的 Java IDE。

## 功能

- 项目管理：创建/打开项目、最近打开列表、从 VCS 克隆、重命名与删除
- 编辑器：语法高亮、行号、代码折叠、查找替换、自动保存、代码补全（关键字/模板/项目符号）、悬停签名、跳转定义
- 构建与运行：`javac` 、Maven、Gradle、`jar` 打包、运行配置（主类/程序参数/VM 参数/环境变量/工作目录）、stdin 输入与进程停止
- 工具窗口：文件树、结构大纲、全局查找、Git 状态/差异/提交/历史、问题（构建错误与 TODO）
- 调试：纯 GO 实现 JDWP 客户端的断点、单步、调用栈、变量树、监视与条件表达式
- Java 分析：纯 GO 实现词法 + LL(k) 递归下降解析器，双投影（AST 结构大纲 / 事件流符号索引），编辑中间态可容错

## 环境要求

- Go 1.27.2+
- `PATH` 中可用的 `javac`、`java`、`javap`、`git`（Java/Maven/Gradle/git 命令可在设置中单独指定）

## 项目构建和运行

```sh
go run .                    # 运行 IDE（需 GUI 环境）
go build -o java-ide .      # 产出本地可执行文件
go tool mygo vet .          # MyGo 静态检查
```
