#!/bin/bash

echo "🚀 Starting Quick Motorcycle CV Tunnel..."
echo ""

if ! curl -s http://localhost:8080 > /dev/null; then
    echo "❌ Go server is not running on localhost:8080"
    echo "Please start it first with: go run server/main.go"
    exit 1
fi

echo "✅ Go server is running, starting tunnel..."
echo ""

cloudflared tunnel --url http://localhost:8080 2>&1 | while IFS= read -r line; do
    echo "$line"
    
    if echo "$line" | grep -q "trycloudflare.com"; then
        URL=$(echo "$line" | grep -o 'https://[^[:space:]]*\.trycloudflare\.com')
        if [ ! -z "$URL" ]; then
            echo ""
            echo "🎉 Your app is now available at: $URL"
            echo "📱 Use this URL on your mobile device to test the camera!"
            echo "🔗 This URL will work as long as this script is running"
            echo ""
        fi
    fi
done
