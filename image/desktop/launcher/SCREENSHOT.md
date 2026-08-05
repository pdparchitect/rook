# screenshot.jpg is missing on purpose

`application.json` references `screenshot.jpg` as the cover and screenshot, but
the file is not committed yet: a real screenshot has to come from a running
session, not a generated placeholder.

To capture it:

```sh
docker build -f image/desktop/Dockerfile -t rook-desktop .
docker run --rm -it --shm-size=1g -p 6901:6901 -p 6902:6902 rook-desktop
```

Open <http://localhost:6901>, let the welcome terminal settle, then save the
frame — either from the browser, or from the desktop bridge's snapshot:

```sh
curl -s http://localhost:6902/preview.jpg -o image/desktop/launcher/screenshot.jpg
```

Raster artwork is tracked with Git LFS in the Launcher ecosystem; add it the
same way if this repo adopts LFS. Until the file exists, Launcher's media
validation will reject the application bundle, so capture it before publishing.
