# Fedora deployment

The GitHub Actions workflow checks out `main` on the runner and streams the source archive to `$HOME/predictboss` over SSH. Fedora does not need to clone the GitHub repository. Keep this directory dedicated to deployment; each deploy replaces its source while preserving `.env` until the new configuration is written.

## One-time Fedora setup

Install Git and Docker Engine with the Docker Compose plugin using Docker's [Fedora installation guide](https://docs.docker.com/engine/install/fedora/), then start Docker:

```bash
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
```

Log out and back in after changing the `docker` group. The deployment account must be able to run `docker compose` without an interactive password prompt.

The workflow creates `$HOME/predictboss` on its first run. The deployment account must own its home directory and have permission to create files there.

Put the company CSV outside the Git checkout. For example, copy it from Windows with:

```powershell
scp "C:\PredictBoss\docs\df_Uid_data_hack.csv" guest@system.onix.fun:~/predictboss-data/df_Uid_data_hack.csv
```

Create `~/predictboss-data` on Fedora first. Set `DOMAIN_CSV_PATH` in the `PRODUCTION_ENV_FILE` GitHub secret to the CSV's absolute path. Set strong, unique production values for every password and API key, `DOMAIN_JWT_KEY` (at least 32 bytes), and `ATLAS_TOKEN` for ClickHouse migrations. Keep `SWAGGER_ENABLED=false` in production. The compose file binds application and storage ports to loopback by default.

Configure the existing host Caddy to proxy your public domain to the frontend. For `system.onix.fun`, add this site block to the server's existing Caddyfile (preserve any other sites already configured):

```caddyfile
system.onix.fun {
    reverse_proxy 127.0.0.1:3000
}
```

The domain must resolve to Fedora, and inbound TCP ports 80/443 must reach the existing Caddy service. Validate and reload its configuration using the server's established Caddy service procedure.

For a manual deployment, clone the repository and create `.env` from the production settings, then run:

```bash
bash deploy/fedora/deploy.sh
```

## SSH key for GitHub Actions

Create a dedicated Ed25519 key pair on an administrator workstation; leave its passphrase empty so Actions can use it non-interactively:

```powershell
ssh-keygen -t ed25519 -C github-actions-predictboss -f "$HOME/.ssh/predictboss_deploy"
```

Add the public key from `predictboss_deploy.pub` to the Fedora deploy account's `~/.ssh/authorized_keys` (mode `600`, with `~/.ssh` mode `700`). Do not copy the private key to Fedora.

Verify the Fedora SSH host key out of band before trusting it. The GitHub secret `FEDORA_KNOWN_HOSTS` must contain the verified `known_hosts` line for the host. Configure these repository or `production` environment secrets in GitHub:

| Secret | Value |
| --- | --- |
| `FEDORA_HOST` | Fedora DNS name, such as `system.onix.fun` |
| `FEDORA_USER` | Dedicated deployment account, such as `guest` |
| `FEDORA_SSH_KEY` | Contents of `predictboss_deploy` (private key) |
| `FEDORA_KNOWN_HOSTS` | Verified `known_hosts` entry for `FEDORA_HOST` |
| `PRODUCTION_ENV_FILE` | Full production `.env` contents in dotenv format (multiline secret) |
| `LLM_API_KEY` | Current LLM provider API key; kept separate for rotation |

Before every deployment, the workflow writes `$HOME/predictboss/.env` over the verified SSH connection with file mode `600`. Put the stable production settings and credentials in `PRODUCTION_ENV_FILE`, replacing placeholders and local defaults. Keep `LLM_API_KEY` out of that file; the workflow adds its value from the separate secret of the same name. Set `DOMAIN_CSV_PATH` to the absolute CSV path on Fedora (for example `/home/guest/predictboss-data/df_Uid_data_hack.csv`); the CSV itself stays on the server and is never sent to GitHub. Keep `SWAGGER_ENABLED=false` and `HOST_BIND_ADDRESS=127.0.0.1`.

Each push to `main` composes the server `.env` from `PRODUCTION_ENV_FILE` and `LLM_API_KEY`, transfers the tracked source archive, and runs the deployment. The workflow keeps one deployment active at a time. Update the LLM credential by changing only the `LLM_API_KEY` GitHub secret; update other production settings through `PRODUCTION_ENV_FILE`. Each deployment replaces the server `.env` with the current composed values.
