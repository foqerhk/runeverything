# Homebrew Tap (staging copy)

将本目录推送到独立 GitHub 仓库：

**`https://github.com/foqerhk/homebrew-tap`**

用户即可：

```bash
brew install foqerhk/tap/runeverything
```

公式文件：[`Formula/runeverything.rb`](Formula/runeverything.rb)

主仓库发版后执行：

```bash
../scripts/build-release.sh 0.1.0
../scripts/sync-homebrew-formula.sh 0.1.0
# 再 commit + push 本 tap 仓库
```
