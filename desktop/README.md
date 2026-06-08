# AllCallAll Desktop

Desktop uses Electron as a thin shell around the Web client.

## Local development

1. Start the web client in `mobile/`:
   - `cd mobile && npm run web`
2. Install desktop dependencies:
   - `cd desktop && npm install`
3. Launch Electron:
   - `cd desktop && npm run dev`

## Checks and packaging

```bash
cd desktop
npm run check
npm run build
```

`npm run build` creates an unpacked Electron app in `desktop/dist` for smoke testing. Use `npm run dist` only when you are ready to create platform installers.

## Notes

- Default web URL: `http://localhost:8081`
- Override with `ALLCALLALL_WEB_URL`
- Override the managed download folder with `ALLCALLALL_DOWNLOAD_DIR`
- Desktop opens to `/meetings` and reuses the Web routes `/rooms/:roomId` and `/conversations/:conversationId`
- The Electron package registers the `allcallall://` scheme; `allcallall://rooms/:roomId` and `allcallall://conversations/:id` are normalized to Web routes
- Push notifications, Web billing, auto-update, and native screen sharing are intentionally out of scope for this thin shell
