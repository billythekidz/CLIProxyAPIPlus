const fs = require('fs');
const filepath = 't:/cliproxyapi/CLIProxyAPIPlus/sdk/api/handlers/claude/code_handlers.go';
let content = fs.readFileSync(filepath, 'utf8');

const bypassFunc = `
// handleNativeWebSearchBypass directly forwards Anthropic requests containing web search triggers
// to copilot-openai's native /v1/messages endpoint, bypassing all translation and streaming it raw.
func (h *ClaudeCodeAPIHandler) handleNativeWebSearchBypass(c *gin.Context, rawJSON []byte) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Streaming not supported",
				Type:    "server_error",
			},
		})
		return
	}

	copilotOpenaiURL := "http://172.21.0.1:8320/v1/messages"
	
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, copilotOpenaiURL, bytes.NewReader(rawJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{Message: err.Error(), Type: "server_error"},
		})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	
	httpClient := &http.Client{}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{Message: err.Error(), Type: "server_error"},
		})
		return
	}
	defer httpResp.Body.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	
	if httpResp.StatusCode >= 400 {
		c.Status(httpResp.StatusCode)
	}

	buf := make([]byte, 4096)
	for {
		n, err := httpResp.Body.Read(buf)
		if n > 0 {
			_, _ = c.Writer.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				log.Errorf("Error reading native bypass stream: %v", err)
			}
			break
		}
	}
}
`;

const interceptBlock = `
	// NATIVE WEB SEARCH BYPASS
	if bytes.Contains(rawJSON, []byte("Perform a web search for the query:")) || 
	   bytes.Contains(rawJSON, []byte("\\"name\\":\\"web_search")) || 
	   bytes.Contains(rawJSON, []byte("\\"name\\":\\"web-search")) || 
	   bytes.Contains(rawJSON, []byte("\\"name\\":\\"websearch")) {
		h.handleNativeWebSearchBypass(c, rawJSON)
		return
	}
`;

content = content.replace(/(streamResult := gjson\.GetBytes.*?streamResult\.Type == gjson\.False \{)/ms, interceptBlock + '\n\t$1');
content = content + '\n' + bypassFunc;

fs.writeFileSync(filepath, content);
console.log('SUCCESS');
