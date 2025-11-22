# Packaging and Releasing `doitdoit`

Here are the best ways to package and release your CLI tool for use on other machines.

## Option 1: `go install` (Easiest for Go developers)

If your other machines have Go installed, you can install directly from the source.

1.  **Push your code** to a public Git repository (e.g., GitHub).
2.  **Run `go install`** on the target machine:

    ```bash
    go install github.com/dtt101/doitdoit@latest
    ```

    This will compile the binary and place it in `$GOPATH/bin` (usually `~/go/bin`), which should be in your `$PATH`. You can then run `doitdoit` anywhere.

## Option 2: Manual Binary Build

If you want to just copy a binary file:

1.  **Build the binary** for your target OS/Architecture.

    **For macOS (Apple Silicon):**
    ```bash
    GOOS=darwin GOARCH=arm64 go build -o doitdoit
    ```

    **For macOS (Intel):**
    ```bash
    GOOS=darwin GOARCH=amd64 go build -o doitdoit
    ```

    **For Linux (amd64):**
    ```bash
    GOOS=linux GOARCH=amd64 go build -o doitdoit
    ```

    **For Windows:**
    ```bash
    GOOS=windows GOARCH=amd64 go build -o doitdoit.exe
    ```

2.  **Copy the binary** (`doitdoit`) to the target machine (e.g., via Dropbox, SCP, USB).
3.  **Move it to a bin directory** (on macOS/Linux):
    ```bash
    mv doitdoit /usr/local/bin/
    ```

## Option 3: GoReleaser (Recommended for Releases)

[GoReleaser](https://goreleaser.com/) automates building binaries for all platforms and creating GitHub Releases.

1.  **Install GoReleaser**:
    ```bash
    brew install goreleaser/tap/goreleaser
    ```

2.  **Initialize**:
    ```bash
    goreleaser init
    ```
    This creates a `.goreleaser.yaml` file.

3.  **Tag a Release**:
    ```bash
    git tag -a v0.1.0 -m "First release"
    git push origin v0.1.0
    ```

4.  **Release**:
    ```bash
    goreleaser release --clean
    ```
    This will build binaries, create archives, and upload them to GitHub Releases automatically.

## Option 4: Homebrew Tap (Advanced)

If you use GoReleaser, you can also generate a Homebrew Tap so users can install with:
```bash
brew install yourusername/tap/doitdoit
```
This requires configuring the `brews` section in `.goreleaser.yaml`.

## Distributing via GitHub (Step-by-Step)

Since you have a local git repository, here is how to get it on GitHub:

1.  **Create a Repository**: Go to [GitHub.com/new](https://github.com/new) and create a repository named `doitdoit`. Do not initialize with README/gitignore (you already have them).

2.  **Push your code**:
    ```bash
    git remote add origin https://github.com/YOUR_USERNAME/doitdoit.git
    git branch -M main
    git add .
    git commit -m "Initial commit"
    git push -u origin main
    ```

3.  **Create a Release**:
    *   **Manual**: Go to your repo -> "Releases" -> "Draft a new release". Tag it `v0.1.0`, upload your binary (built in Option 2), and publish.
    *   **Automated**: Use GoReleaser (Option 3) to do this automatically.

4.  **How Users Install**:
    *   **From Source**: `go install github.com/YOUR_USERNAME/doitdoit@latest`
    *   **From Binary**: Download `doitdoit` from the Releases page.
