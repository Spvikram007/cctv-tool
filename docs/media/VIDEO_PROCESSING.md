# CCTV SENTINEL UNIVERSAL — VIDEO PROCESSING & TRANSCODING
# ISOLATED PIPELINE & MULTI-VIEWER FANOUT

**Product**: CCTV SENTINEL UNIVERSAL  
**Module**: Video Processing Engine  
**Package**: `backend/pkg/media/streamhub`  
**Distribution**: Commercial CCTV Technician Platform  

---

## 1. Engine Architecture & Isolation

Modern web browsers do not natively support RTSP or raw H.265 video playback without heavy third-party plugins, WebAssembly decoders, or vendor ActiveX components.

CCTV Sentinel Universal implements an **Isolated Media Transcoding Pipeline**:

1. **Subprocess Isolation**:
   - Spawns background worker instances using `exec.Command`.
   - Arguments are passed as direct slices (`[]string`), preventing shell injection, command concatenation, or escaping bugs.
2. **Standard Output Streaming Pipe**:
   - FFmpeg outputs continuous raw MJPEG frames to standard output (`pipe:1`).
   - The Go backend reads stdout directly via `io.Reader`.
3. **In-Memory Frame Extractor**:
   - Detects JPEG start-of-image (`0xFF 0xD8`) and end-of-image (`0xFF 0xD9`) magic markers.
   - Slices valid single frames into memory buffers.
4. **Non-Blocking Viewer Fanout**:
   - Broadcasts completed frames across registered viewer channels (`chan []byte`).
   - If a viewer connection lags, frames are safely dropped for that single viewer without stalling the media pipeline or other technicians.

---

## 2. Process Command Specification

```bash
ffmpeg.exe \
  -rtsp_transport tcp \
  -i "rtsp://username:password@192.168.31.103:554/live/ch0" \
  -f image2pipe \
  -vcodec mjpeg \
  -q:v 3 \
  -r 20 \
  pipe:1
```

### Parameter Breakdown:
- `-rtsp_transport tcp`: Forces interleaved TCP transmission, preventing packet loss over busy or congested switch links.
- `-f image2pipe`: Formats image sequence into a continuous stream pipe.
- `-vcodec mjpeg`: Decodes H.264/H.265 in hardware/software and encodes frames as JPEG.
- `-q:v 3`: High visual quality (scale 1-31, where 1 is best) with low CPU overhead.
- `-r 20`: Caps frame rate at 20 FPS to prevent bandwidth saturation during 16-camera grid views.
- `pipe:1`: Emits data to stdout for memory capture.

---

## 3. Snapshot Engine Integration

The Genuine Snapshot Engine (`backend/pkg/media/snapshot/engine.go`) enforces strict image integrity:

1. **Vendor Adapter First**:
   - Attempts direct authenticated snapshot via ISAPI (`/ISAPI/Streaming/channels/101/picture`) or CGI (`/cgi-bin/snapshot.cgi?channel=1`).
2. **ONVIF Fallback**:
   - Queries `GetSnapshotUri` and retrieves frame via authenticated HTTP GET.
3. **FFmpeg Stream Frame-Grab Fallback**:
   - If vendor endpoints fail or return placeholder SVGs, FFmpeg connects to the RTSP stream and extracts a single high-resolution keyframe (`-vframes 1`).
4. **Magic Byte Validation**:
   - Enforces genuine JPEG (`0xFF 0xD8 0xFF`) or PNG (`0x89 0x50 0x4E 0x47`) headers.
   - Enforces a minimum payload size of 1024 bytes (1 KB).
   - Explicitly rejects `<svg` and `<html` response payloads.
5. **Persistence**:
   - Saves verified snapshot frames to disk: `./data/snapshots/{deviceID}/{deviceID}_{timestamp}.jpg`.
   - Maintains a queryable in-memory history of the last 50 snapshots per camera.
