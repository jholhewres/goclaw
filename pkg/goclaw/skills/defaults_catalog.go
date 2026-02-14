// Package skills – defaults_catalog.go holds the actual default skill templates.
// These are the SKILL.md contents embedded in the binary.
package skills

// defaultSkillList is the list of all default skill templates.
// nolint: lll
var defaultSkillList = []DefaultSkill{
	{
		Name:        "web-search",
		Label:       "🌐 Web Search — search the web via Brave API or DuckDuckGo",
		Description: "Web search via Brave Search API or DuckDuckGo",
		Content: "---\nname: web-search\ndescription: \"Search the web for current information using Brave Search API or DuckDuckGo\"\n---\n# Web Search\n\nYou can search the web for current information.\n\n## Using Brave Search API (preferred, if BRAVE_API_KEY is available)\n\n```bash\n# Web search\ncurl -s \"https://api.search.brave.com/res/v1/web/search?q=QUERY&count=5\" \\\n  -H \"Accept: application/json\" \\\n  -H \"X-Subscription-Token: $BRAVE_API_KEY\" | jq '.web.results[] | {title, url, description}'\n\n# News search\ncurl -s \"https://api.search.brave.com/res/v1/web/search?q=QUERY&count=5&freshness=week&news=true\" \\\n  -H \"Accept: application/json\" \\\n  -H \"X-Subscription-Token: $BRAVE_API_KEY\" | jq '.web.results[] | {title, url, description}'\n```\n\n## Using DuckDuckGo (no API key needed, fallback)\n\n```bash\ncurl -s \"https://html.duckduckgo.com/html/?q=QUERY\" | grep -oP 'class=\"result__a\"[^>]*href=\"\\K[^\"]+' | head -5\n```\n\n## Tips\n- URL-encode the query (replace spaces with +).\n- Use freshness parameter for time-filtered results: day, week, month.\n- Be specific in queries for better results.\n- Check if BRAVE_API_KEY is set; if not, fall back to DuckDuckGo.\n- Combine with web_fetch to read the full content of interesting results.\n",
	},
	{
		Name:        "web-fetch",
		Label:       "📄 Web Fetch — fetch and extract readable content from URLs",
		Description: "Fetch URL content and extract readable text/markdown",
		Content: "---\nname: web-fetch\ndescription: \"Fetch URL content and extract readable text\"\n---\n# Web Fetch\n\nYou can fetch and read the content of any URL.\n\n## Fetching a web page\n\n```bash\ncurl -sL \"URL\" | sed 's/<[^>]*>//g' | sed '/^$/d' | head -200\nreadable \"URL\" 2>/dev/null || curl -sL \"URL\" | sed 's/<[^>]*>//g' | sed '/^$/d' | head -200\n```\n\n## Fetching JSON APIs\n\n```bash\ncurl -s \"API_URL\" -H \"Accept: application/json\" | jq '.'\n```\n\n## Tips\n- Always use -sL (silent + follow redirects).\n- For large pages, pipe through head -N to limit output.\n- Strip HTML tags with sed for readability.\n- Respect robots.txt and rate limits.\n",
	},
	{
		Name:        "github",
		Label:       "🐙 GitHub — issues, PRs, releases, CI via gh CLI",
		Description: "Full GitHub integration via gh CLI",
		Content: "---\nname: github\ndescription: \"GitHub integration via gh CLI\"\n---\n# GitHub\n\nYou can interact with GitHub using the gh CLI.\n\n## Common operations\n\n```bash\ngh repo list --limit 10\ngh issue list -R OWNER/REPO --limit 10\ngh issue create -R OWNER/REPO --title \"TITLE\" --body \"BODY\"\ngh pr list -R OWNER/REPO --limit 10\ngh pr create -R OWNER/REPO --title \"TITLE\" --body \"BODY\"\ngh pr merge NUMBER -R OWNER/REPO --squash\ngh release list -R OWNER/REPO --limit 5\ngh release create TAG -R OWNER/REPO --title \"TITLE\" --notes \"NOTES\"\ngh run list -R OWNER/REPO --limit 5\n```\n\n## Tips\n- Use -R OWNER/REPO to target a specific repo.\n- Use --json for structured output.\n- Check if gh is authenticated: gh auth status\n",
	},
	{
		Name:        "weather",
		Label:       "🌤  Weather — forecasts via wttr.in (no API key needed)",
		Description: "Weather information and forecasts (no API key required)",
		Content: "---\nname: weather\ndescription: \"Weather information and forecasts using wttr.in\"\n---\n# Weather\n\nYou can check weather using wttr.in (no API key needed).\n\n## Current weather\n\n```bash\ncurl -s \"wttr.in/CITY?format=3\"\ncurl -s \"wttr.in/CITY?format=%l:+%c+%t+%h+%w+%p\"\ncurl -s \"wttr.in/CITY?lang=pt\"\n```\n\n## JSON format\n\n```bash\ncurl -s \"wttr.in/CITY?format=j1\" | jq '{\n  location: .nearest_area[0].areaName[0].value,\n  temp_c: .current_condition[0].temp_C,\n  feels_like: .current_condition[0].FeelsLikeC,\n  humidity: .current_condition[0].humidity,\n  description: .current_condition[0].weatherDesc[0].value,\n  wind_kmph: .current_condition[0].windspeedKmph\n}'\n```\n\n## Tips\n- Replace CITY with the city name (use + for spaces).\n- Use lang=pt for Portuguese.\n- wttr.in supports airport codes (GRU, JFK).\n",
	},
	{
		Name:        "summarize",
		Label:       "📊 Summarize — summarize URLs, articles, and text",
		Description: "Summarize URLs, articles, videos, and long texts",
		Content: "---\nname: summarize\ndescription: \"Summarize URLs, articles, and long texts\"\n---\n# Summarize\n\nYou can summarize web pages, articles, and long texts.\n\n## Summarizing a URL\n\n```bash\ncurl -sL \"URL\" | sed 's/<[^>]*>//g' | sed '/^$/d' | head -500\n```\n\nThen summarize the extracted text using your reasoning capabilities.\n\n## YouTube videos\n\n```bash\nyt-dlp --write-auto-subs --skip-download --sub-lang pt,en -o \"/tmp/%(id)s\" \"VIDEO_URL\" 2>/dev/null\ncat /tmp/*.vtt 2>/dev/null | grep -v \"^[0-9]\" | grep -v \"^$\" | grep -v \"WEBVTT\" | grep -v \"-->\" | sort -u | head -300\n```\n\n## Tips\n- Break long texts into sections.\n- Ask the user what detail level they want.\n- Preserve key facts, names, dates.\n",
	},
	{
		Name:        "timer",
		Label:       "⏱️  Timer — timers, alarmes e Pomodoro em segundo plano",
		Description: "Timers, alarms, and Pomodoro sessions",
		Content: "---\nname: timer\ndescription: \"Set timers, alarms, and Pomodoro sessions\"\n---\n# Timer\n\nSet timers that run in background.\n\n## Quick timers\n\n```bash\nsleep 300 && echo \"⏰ Timer de 5 minutos finalizado!\"\nsleep 600 && echo \"⏰ Hora de verificar o forno!\"\nsleep 30 && echo \"⏰ 30 segundos!\"\n```\n\nRun in background mode so user can keep chatting.\n\n## Pomodoro\n\n```bash\nsleep 1500 && echo \"🍅 Pomodoro finalizado! Pausa de 5 min.\"\nsleep 300 && echo \"🔔 Pausa acabou! Volte ao trabalho.\"\n```\n\n## Tips\n- Always run in background.\n- Convert: \"5 minutos\" = sleep 300.\n- For recurring timers, use the scheduler.\n",
	},
	{
		Name:        "reminders",
		Label:       "🔔 Reminders — lembretes com data e hora",
		Description: "Time-based reminders with scheduling",
		Content: "---\nname: reminders\ndescription: \"Create and manage time-based reminders\"\n---\n# Reminders\n\nCreate reminders using the scheduler (cron_add).\n\n## Creating reminders\n\n```bash\ncron_add --id \"rem-123\" --schedule \"0 15 14 2 *\" --payload \"📋 Reunião às 15h\"\ncron_add --id \"daily-water\" --schedule \"0 9 * * *\" --payload \"💧 Beber água!\"\ncron_add --id \"standup\" --schedule \"30 8 * * 1-5\" --payload \"🏃 Standup em 30min!\"\ncron_list\ncron_remove --id \"rem-123\"\n```\n\n## Natural language → cron\n| User says | Cron |\n|-----------|------|\n| todo dia 8h | 0 8 * * * |\n| seg a sex 9h | 0 9 * * 1-5 |\n| toda segunda | 0 9 * * 1 |\n\n## Tips\n- Generate unique IDs for each reminder.\n- For < 1 hour, use the timer skill instead.\n- Always confirm time with user before creating.\n",
	},
	{
		Name:        "notes",
		Label:       "📝 Notes — notas rápidas, listas e ideias",
		Description: "Quick notes, lists, and ideas stored locally",
		Content: "---\nname: notes\ndescription: \"Quick notes, lists, and ideas — stored as local markdown\"\n---\n# Notes\n\nSave and manage notes as markdown files in ~/.goclaw/notes/.\n\n## Creating notes\n\n```bash\nmkdir -p ~/.goclaw/notes\ncat > ~/.goclaw/notes/$(date +%Y%m%d-%H%M%S)-note.md << 'EOF'\n# Quick note\nContent here.\nEOF\n```\n\n## Reading & searching\n\n```bash\nls -lt ~/.goclaw/notes/ | head -20\ncat ~/.goclaw/notes/shopping-list.md\ngrep -rl \"TERM\" ~/.goclaw/notes/\n```\n\n## Tips\n- Use descriptive filenames.\n- Checkboxes: - [ ] todo, - [x] done.\n- Read back after creating for confirmation.\n",
	},
	{
		Name:        "image-gen",
		Label:       "🎨 Image Generation — generate images from text descriptions (DALL-E / GPT-image)",
		Description: "Generate images from text prompts using DALL-E 3 or GPT-image models",
		Content:     "---\nname: image-gen\ndescription: \"Generate images from text descriptions using DALL-E or GPT-image models\"\n---\n# Image Generation\n\nGenerate images from text descriptions using OpenAI's DALL-E 3 or gpt-image-1.\n\n## Requirements\n- OPENAI_API_KEY or GOCLAW_API_KEY environment variable set\n- Optional: GOCLAW_IMAGE_MODEL to choose model (default: dall-e-3)\n\n## Usage\nThe assistant will automatically use the generate_image tool when you ask for images.\n\nExamples:\n- \"Create an image of a sunset over the ocean\"\n- \"Generate a logo for a tech startup\"\n- \"Draw a cartoon cat wearing a hat\"\n\n## Parameters\n- prompt: Detailed description of the image\n- size: 1024x1024 (default), 1024x1792 (portrait), 1792x1024 (landscape)\n- quality: standard or hd\n- style: vivid (default) or natural (dall-e-3 only)\n",
	},
	{
		Name:        "translate",
		Label:       "🌍 Translate — traduções entre idiomas",
		Description: "Translate text between languages",
		Content: "---\nname: translate\ndescription: \"Translate text between any languages\"\n---\n# Translate\n\nTranslate text using your multilingual capabilities.\n\n## Built-in (preferred)\nAs a multilingual LLM, translate directly when asked.\n\n## External verification (LibreTranslate)\n\n```bash\ncurl -s -X POST \"https://libretranslate.com/translate\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"q\": \"TEXT\", \"source\": \"en\", \"target\": \"pt\"}' | jq -r '.translatedText'\n```\n\n## Tips\n- For casual translations, use built-in capabilities.\n- Preserve formatting during translation.\n- Don't translate proper nouns unless asked.\n",
	},
	{
		Name:        "claude-code",
		Label:       "🤖 Claude Code — full-stack coding assistant via Claude Code CLI",
		Description: "Advanced coding capabilities powered by Claude Code: edit, review, commit, PR, deploy, test, refactor",
		Content: "---\nname: claude-code\ndescription: \"Full-stack coding assistant powered by Claude Code CLI\"\n---\n# Claude Code Integration\n\nThis skill integrates with the Claude Code CLI to provide advanced software development capabilities directly from your chat.\n\n## Requirements\n- Claude Code CLI installed: `npm install -g @anthropic-ai/claude-code`\n- Authenticated: `claude setup-token` or `claude login`\n\n## What Claude Code Can Do\n- **Code editing**: Read, create, edit, refactor files in any language\n- **Git**: Status, diff, commit, branch, merge, rebase\n- **Pull Requests**: Create, review, merge PRs via gh/glab CLI\n- **Testing**: Run tests, analyze failures, fix bugs\n- **Build & Deploy**: Build projects, Docker, server config, deployment\n- **Search**: Grep, glob, semantic search across codebase\n- **Multi-file refactoring**: Rename, restructure, migrate\n\n## Configuration\n- `GOCLAW_CLAUDE_CODE_MODEL`: Default model (sonnet, opus, haiku). Default: sonnet\n- `GOCLAW_CLAUDE_CODE_BUDGET`: Default budget per task in USD. Default: 1.0\n- `GOCLAW_CLAUDE_CODE_TIMEOUT_MIN`: Timeout in minutes. Default: 5\n\n## Usage Flow\n1. Register/activate a project: `project-manager_register` + `project-manager_activate`\n2. Send coding tasks: `claude-code_execute` with a detailed prompt\n3. For multi-step tasks: use `continue_session=true`\n\n## Tips\n- Be specific in your prompts\n- For read-only analysis, set `permission_mode=plan`\n- Monitor costs via the response footer\n- Use `allowed_tools` to restrict what Claude Code can do\n",
	},
	{
		Name:        "project-manager",
		Label:       "📂 Project Manager — manage development projects and workspaces",
		Description: "Register, scan, activate, and inspect development projects for coding skills",
		Content: "---\nname: project-manager\ndescription: \"Manage development projects for coding skills\"\n---\n# Project Manager\n\nManage your development projects so coding skills know where to operate.\n\n## Operations\n- **Register**: Add a project by path (auto-detects language, framework, commands)\n- **Scan**: Discover projects in a directory (e.g. ~/Workspace)\n- **Activate**: Set the active project for your session\n- **Info**: View project details (language, framework, git, commands)\n- **Tree**: View file structure\n\n## Usage\n1. Scan your workspace: `project-manager_scan directory=~/Workspace register=true`\n2. Activate a project: `project-manager_activate project_id=my-project`\n3. Use coding skills (claude-code) — they operate on the active project\n\n## Auto-Detection\nDetects: Go, TypeScript, JavaScript, Python, PHP, Rust, Ruby, Java, Kotlin, Dart, C++, Elixir, Swift\nFrameworks: Laravel, Next, React, Vue, Django, FastAPI, Gin, Rails, and more\n",
	},
}
