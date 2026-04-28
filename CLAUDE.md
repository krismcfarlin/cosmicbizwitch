# Cosmic Biz Witch — kathmandu-v2

Go + PocketBase backend. Deployed at https://server.cosmicbizwitch.com

## Credentials (never ask the user for these)

### PocketBase (superuser)
- Email: krismcfarlin@gmail.com
- Password: super1234bad
- API: https://server.cosmicbizwitch.com/api/...

### ClickFunnels
- API Key: g-wmRl3iXx9x6TIklIxODr0A8KoDGqw0TXoowJpJ2Rc
- Subdomain: cosmicbizwitchllc
- Workspace ID: 39498
- Base URL: https://cosmicbizwitchllc.myclickfunnels.com/api/v2
- Contacts endpoint: /workspaces/39498/contacts
- Pagination: cursor-based via `pagination-next` response header and `after` query param, 20/page max

### Telegram Bot
- Token: 7943753215:AAGRyGBrIh9vvyMrAiLSyr9lET-ZYkUWwCo

### Google OAuth
- Client ID: see app_settings collection (key: google_client_id)
- Client Secret: see app_settings collection (key: google_client_secret)

### SSH (production server)
- Host: root@159.223.157.70
- Key: ~/.ssh/githubkey2

## Key PocketBase collections
- `contacts` — main contact list (first_name, last_name, email, phone, date_added, tags, contact_id=CF id)
- `cf_contacts` — CF sync mirror (cf_id, email_address, first_name, last_name, tags, etc.)
- `telegram_messages` — raw Telegram webhook payloads (SQLite table, not PB collection)
- `app_settings` — key/value store for all credentials and config

## MCP proxy (cosmicbizwitch)
Configured in ~/.claude.json under the project path. Binary at ~/.local/bin/cbw-mcp.
Tools: pb_schema, pb_list, pb_get, pb_create, pb_update, pb_delete, cf_list_contacts, cf_get_contact, cf_create_contact, cf_update_contact, cf_sync_contacts.

## CF pagination (important)
CF API returns max 20 contacts per page. Always paginate using `pagination-next` response header value as `after` param. Do NOT stop when len(page) < per_page — stop when `pagination-next` header is absent.

## Deploy
```
bash deploy.sh
```
Uses SSH key ~/.ssh/githubkey2 to rsync + restart on production server.
