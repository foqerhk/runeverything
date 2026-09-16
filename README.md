# RunEverything APT repository

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
```

Or add the source manually (after publishing this tree to GitHub Pages):

```bash
echo 'deb [trusted=yes] https://foqerhk.github.io/runeverything/apt stable main' | sudo tee /etc/apt/sources.list.d/runeverything.list
sudo apt update
sudo apt install runeverything
```
