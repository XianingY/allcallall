# AllCallAll Desktop

Desktop uses Electron as a thin shell around the Web client.

## Local development

1. Start the Web client in `web/`:
   - `cd web && npm install && npm run dev`
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

`npm run check` also validates desktop deep-link normalization without starting Electron. This covers:

- `allcallall://rooms/:roomId`
- `allcallall://meetings/:roomId`
- `allcallall://conversations/:conversationId`
- `allcallall://meetings`
- Web routes such as `/meetings/:roomId`
- rejection of non-AllCallAll external route targets

## Notes

- Default web URL: `http://localhost:5173`
- Override with `ALLCALLALL_WEB_URL`
- Override the managed download folder with `ALLCALLALL_DOWNLOAD_DIR`
- Default managed download folder: `~/Downloads/AllCallAll`
- Desktop opens to `/meetings` and reuses the Web routes `/meetings/:roomId` and `/conversations/:conversationId`
- The Electron package registers the `allcallall://` scheme; legacy `allcallall://rooms/:roomId` links are normalized to `/meetings/:roomId`
- Auto-update and native screen sharing are intentionally out of scope for this thin shell. Billing and Web Push are handled by the loaded Web app when its runtime config is present.
