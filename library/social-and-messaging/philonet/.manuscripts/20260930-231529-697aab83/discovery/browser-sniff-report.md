# Browser-Sniff Report: philonet
Backend: Chrome DevTools MCP against the user's existing logged-in Chrome session (philonet.ai). Host: api.typepilot.app. Auth: Authorization Bearer JWT (localStorage accessToken); env PHILONET_TOKEN in the CLI. Runtime: standard HTTP (no bot challenge).
Goal flow: browse feed -> search -> read thought thread -> profile/stats -> notifications/requests.
Pages: /home, /search?q=ai, /myconversations, /notifications, /profile, /conversations/{article}/{comment}.
Method: passive network listing per page + in-page fetch hooks, then read-only replay of 38 calls (token headers stripped, emails redacted, arrays trimmed; response bodies were stripped from the archived capture) into browser-sniff-capture.json.
Not exercised (write ops found in JS bundle): addcommentnew, starinsight, bookmarkconversation, feed/{id}/insightful|reactions, friend/request, thinkingtime, storehighlight, summarise*, upload-pdf, room/article (add link).
