# Security

## Reporting a vulnerability

Please report security issues privately through GitHub's security-advisory
feature rather than opening a public issue.

## Webhook credentials

Discord webhook URLs grant permission to post to their channel. The application
stores the configured URL only in the local `config.json`, which is excluded
from Git. Never attach that file to a bug report. Rotate the webhook in Discord
immediately if its URL is exposed.
