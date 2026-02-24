---
name: media
description: "Handle images, audio, and document files for messaging channels"
trigger: automatic
---

# Media Tools

Process and send media files (images, audio, documents) through messaging channels like WhatsApp.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│  INPUT MEDIA  │                    │ OUTPUT MEDIA  │
├───────────────┤                    ├───────────────┤
│describe_image │                    │  send_image   │
│transcribe_audio                    │  send_audio   │
└───────┬───────┘                    │ send_document │
        │                            └───────┬───────┘
        │                                    │
        ▼                                    ▼
┌───────────────┐                    ┌───────────────┐
│  AI Analysis  │                    │   Messaging   │
│  (Vision/STT) │                    │   Channel     │
└───────────────┘                    └───────────────┘
```

## Tools

| Tool | Action | Direction |
|------|--------|-----------|
| `describe_image` | Analyze image with AI vision | Input |
| `transcribe_audio` | Convert audio to text | Input |
| `send_image` | Send image to user | Output |
| `send_audio` | Send audio file | Output |
| `send_document` | Send PDF/document | Output |

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
# Output:
# The image shows a green fern plant in a white ceramic pot.
# The plant appears healthy with several fronds. It's sitting
# on a wooden table near a window with natural light.
```

### Audio Transcription

```bash
transcribe_audio(audio_path="/tmp/voice.ogg")
# Output:
# "Olá, gostaria de saber se vocês entregam na minha região.
#  Moro no centro da cidade."
```

## Sending Media

### Send Image

```bash
send_image(
  image_path="/tmp/chart.png",
  caption="Gráfico de vendas do mês"
)
# Output: Image sent successfully to whatsapp:5511999999
```

### Send Audio

```bash
send_audio(
  audio_path="/tmp/response.mp3",
  caption="Resposta em áudio"
)
# Output: Audio sent successfully
```

### Send Document

```bash
send_document(
  document_path="/tmp/report.pdf",
  caption="Relatório Mensal - Fevereiro 2026",
  filename="relatorio-fev-2026.pdf"
)
# Output: Document sent successfully
```

## Supported Formats

| Type | Formats | Max Size |
|------|---------|----------|
| Images | PNG, JPG, JPEG, GIF, WEBP | 5MB |
| Audio | MP3, OGG, WAV, M4A | 16MB |
| Documents | PDF, DOC, DOCX, XLS, XLSX | 100MB |

## Common Patterns

### Image Analysis Response
```bash
# User sends photo of a plant

# 1. Analyze the image
describe_image(image_path="/tmp/plant.jpg")
# Output: "A healthy monstera plant with large green leaves..."

# 2. Respond with analysis
send_message("That's a beautiful Monstera deliciosa! It likes bright indirect light and watering when the top inch of soil is dry.")
```

### Voice Message Flow
```bash
# User sends voice message

# 1. Transcribe
transcribe_audio(audio_path="/tmp/voice.ogg")
# Output: "What's the weather like today?"

# 2. Process and respond
send_message("It's currently sunny and 24°C in your area!")
```

### Generate and Send Chart
```bash
# 1. Generate visualization
bash(command="python scripts/generate_chart.py --output /tmp/sales.png")

# 2. Send to user
send_image(
  image_path="/tmp/sales.png",
  caption="Vendas por categoria - Últimos 30 dias"
)
```

### Create and Send Report
```bash
# 1. Generate PDF report
bash(command="pandoc report.md -o /tmp/report.pdf --pdf-engine=weasyprint")

# 2. Send to user
send_document(
  document_path="/tmp/report.pdf",
  caption="Relatório de Atividades",
  filename="relatorio-atividades.pdf"
)
```

### Audio Response
```bash
# 1. Generate audio (TTS)
bash(command="tts --text 'Your order has been confirmed' --output /tmp/confirm.mp3")

# 2. Send audio
send_audio(
  audio_path="/tmp/confirm.mp3",
  caption="Confirmação de pedido"
)
```

## Troubleshooting

### "File not found"

**Cause:** Path incorrect or file doesn't exist.

**Debug:**
```bash
# Check file exists
bash(command="ls -la /tmp/photo.jpg")
```

### "Unsupported format"

**Cause:** File format not supported by channel.

**Solution:**
```bash
# Convert format
bash(command="ffmpeg -i input.wav output.mp3")
send_audio(audio_path="/tmp/output.mp3")
```

### "File too large"

**Cause:** Exceeds channel size limit.

**Solution:**
```bash
# Compress image
bash(command="convert large.png -resize 50% -quality 85 small.jpg")

# Compress PDF
bash(command="gs -sDEVICE=pdfwrite -dPDFSETTINGS=/ebook -o small.pdf large.pdf")
```

### "Transcription failed"

**Cause:** Audio quality poor or language not supported.

**Debug:**
```bash
# Check audio file
bash(command="ffprobe /tmp/voice.ogg")

# Convert to better format
bash(command="ffmpeg -i voice.ogg -ar 16000 voice-clean.wav")
```

### "Image analysis failed"

**Cause:** Image corrupted or format issue.

**Debug:**
```bash
# Verify image
bash(command="file /tmp/photo.jpg")
bash(command="identify /tmp/photo.jpg")
```

## Tips

- **Add captions**: Provides context for media
- **Use descriptive filenames**: Easier to identify
- **Check file sizes**: Large files may fail
- **Compress when needed**: Faster upload/sending
- **Verify paths**: Ensure files exist before sending
- **Handle formats**: Convert unsupported formats

## Workflow Examples

### Customer Support with Photo
```bash
# Customer sends photo of damaged product

# 1. Analyze image
describe_image(image_path="/tmp/damage.jpg")
# Output: "Shows a cracked screen on a smartphone..."

# 2. Create support ticket
send_message("I can see the screen damage. I'll process a replacement for you.")

# 3. Generate return label
bash(command="python scripts/generate_label.py --order 12345 --output /tmp/label.pdf")

# 4. Send label
send_document(
  document_path="/tmp/label.pdf",
  caption="Return shipping label - Order #12345",
  filename="return-label.pdf"
)
```

### Report Generation Workflow
```bash
# 1. Gather data
bash(command="python scripts/analyze_sales.py > /tmp/report.md")

# 2. Convert to PDF
bash(command="pandoc /tmp/report.md -o /tmp/sales-report.pdf")

# 3. Generate summary chart
bash(command="python scripts/chart.py --output /tmp/chart.png")

# 4. Send both
send_image(image_path="/tmp/chart.png", caption="Resumo de vendas")
send_document(
  document_path="/tmp/sales-report.pdf",
  caption="Relatório completo",
  filename="sales-report.pdf"
)
```

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| No caption | Always add descriptive caption |
| Sending huge files | Compress before sending |
| Wrong path | Verify with `ls` first |
| Unsupported format | Convert to supported format |
| Missing filename param | Set clear filename for documents |
