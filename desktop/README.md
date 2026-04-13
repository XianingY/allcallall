# AllCallAll Desktop

Desktop uses Electron as a thin shell around the Web client.

## Local development

1. Start the web client in `mobile/`:
   - `cd /Users/byzantium/github/allcallall/mobile && npm run web`
2. Install desktop dependencies:
   - `cd /Users/byzantium/github/allcallall/desktop && npm install`
3. Launch Electron:
   - `cd /Users/byzantium/github/allcallall/desktop && npm run dev`

## Notes

- Default web URL: `http://localhost:8081`
- Override with `ALLCALLALL_WEB_URL`
- Desktop reuses web routes and deep-link semantics
