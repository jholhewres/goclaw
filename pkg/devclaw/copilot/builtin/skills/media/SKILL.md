---
name: media
description: "Handle images, audio, and document files for messaging channels"
trigger: automatic
---

# Media Tools

Process and send media files (images, audio, documents) through messaging channels like WhatsApp.

## Tools

| Tool | Action |
|------|--------|
| `describe_image` | Analyze image content with AI vision |
| `transcribe_audio` | Convert audio to text |
| `send_image` | Send image to user/channel |
| `send_audio` | Send audio file |
| `send_document` | Send document/PDF |

## When to Use

| Tool | When |
|------|------|
| `describe_image` | User sends image, needs analysis |
| `transcribe_audio` | User sends voice message |
| `send_image` | Visual response needed |
| `send_audio` | Voice response, music |
| `send_document` | PDFs, reports, files |

## Input Processing

### Image Analysis
```bash
describe_image(image_path="/tmp/photo.jpg")
# Output: Description of image contents...
```

### Audio Transcription
```bash
transcribe_audio(audio_path="/tmp/voice.ogg")
# Output: Transcribed text from audio...
```

## Sending Media

### Send Image
```bash
send_image(
  image_path="/tmp/chart.png",
  caption="Monthly sales report"
)
# Output: Image sent successfully
```

### Send Audio
```bash
send_audio(
  audio_path="/tmp/response.mp3",
  caption="Voice note"
)
# Output: Audio sent successfully
```

### Send Document
```bash
send_document(
  document_path="/tmp/report.pdf",
  caption="Q4 Financial Report",
  filename="report.pdf"
)
# Output: Document sent successfully
```

## Workflow Examples

### Analyze and Respond to Image
```bash
# User sends photo of a plant
1. describe_image(image_path="/tmp/plant.jpg")
   → Output: "A green fern in a ceramic pot..."

2. send_message("That's a beautiful fern! It needs indirect light and regular watering.")
```

### Voice Message Flow
```bash
# User sends voice message
1. transcribe_audio(audio_path="/tmp/voice.ogg")
   → Output: "What's the weather today?"

2. # Process the question and respond
3. send_message("It's sunny and 75°F today!")
```

### Send Report as PDF
```bash
# Generate report
1. # Create PDF file...

2. send_document(
     document_path="/tmp/monthly-report.pdf",
     caption="Monthly Report - February 2026",
     filename="february-report.pdf"
   )
```

## Supported Formats

| Type | Formats |
|------|---------|
| Images | PNG, JPG, JPEG, GIF, WEBP |
| Audio | MP3, OGG, WAV, M4A |
| Documents | PDF, DOC, DOCX, XLS, XLSX |

## Best Practices

| Practice | Reason |
|----------|--------|
| Add captions | Context for the media |
| Use descriptive filenames | Easier to identify |
| Check file sizes | Large files may fail |
| Compress when needed | Faster upload/sending |

## Common Patterns

### Visual Response
```bash
# Generate a chart
bash("python generate_chart.py --output /tmp/chart.png")

# Send it
send_image(image_path="/tmp/chart.png", caption="Sales trend this week")
```

### Document Delivery
```bash
# Create document
bash("pandoc report.md -o /tmp/report.pdf")

# Deliver to user
send_document(
  document_path="/tmp/report.pdf",
  caption="Here's your requested report",
  filename="analysis-report.pdf"
)
```

## Important Notes

| Note | Reason |
|------|--------|
| Files must exist | Verify path before sending |
| Channel support varies | Not all channels support all media |
| Size limits apply | Check channel documentation |
| Captions are optional | But recommended for context |
