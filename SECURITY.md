# Security Policy

## Reporting Security Issues

If you discover a security vulnerability in this project, please report it by emailing the maintainers. Please do not disclose security vulnerabilities publicly until they have been addressed.

## Security Considerations

### API Key Storage

- API keys are encrypted using AES-256-GCM before storage
- The encryption key is hardcoded to ensure consistency across deployments
- **Important**: Users should always use strong, unique passwords for the admin interface

### Environment Variables

This project uses environment variables for configuration. Never commit files containing real credentials:

- `.env` - Local environment configuration (gitignored)
- Create your own `.env` from `env.example`
- All sensitive values should be set via environment variables

### Deployment Security

- Always use HTTPS in production
- Change default passwords immediately after deployment
- Restrict database file access (`data/proxy.db`)
- Review firewall rules and network policies

### Best Practices

1. **API Keys**: Never commit real API keys to version control
2. **Database**: The SQLite database file is gitignored - ensure it has proper file permissions
3. **Logs**: Log files are gitignored - review logs for sensitive data before sharing
4. **Updates**: Keep dependencies updated to patch security vulnerabilities

## Supported Versions

Currently supported versions with security updates:

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Security Features

- JWT-based authentication
- API key encryption at rest
- Request/response logging (be careful with sensitive data)
- CORS configuration
- Rate limiting (recommended to add)
