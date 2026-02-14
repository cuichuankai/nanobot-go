---
name: assistant-pic
description: Generate and send a selfie of the assistant's photo using the 'media-generation' tool.
nanobot:
  always: true
---

# Assistant Selfie Generation

This skill guides you to use the built-in `media-generation` tool to regenerate the assistant's photo by the reference image and send it to the user.

## Triggers

- User asks for a selfie/photo/pic of you/assistant.
- User asks "what are you doing?" or "where are you?" and implies a visual response.
- User describes a context: "send a pic wearing...", "send a pic at..."
- User wants the assistant to appear in a specific outfit, location, or situation

## **Reference Image**:
```
 `https://share.cheersucloud.com/apps/files_sharing/publicpreview/7XYomPrSrbrNYzZ?file=/&fileId=136606&x=1920&y=1080&a=true&etag=183eaedb94cd1a5685372f073900dc08`
 ```

## Procedure

1.  **Construct Prompt**: Extract the context from the user's request (e.g., "at the beach", "wearing a red dress").
2.  **Call Tool**: Use the `media-generation` tool.
    *   `task`: "image-to-image"
    *   `prompt`: "make a pic of this person, but [CONTEXT]. the person is taking a mirror selfie" (or use "a close-up selfie..." for direct mode)
    *   `model`: "Qwen/Qwen-Image-Edit-2509" (default)
    *   `image_url`: "https://share.cheersucloud.com/apps/files_sharing/publicpreview/7XYomPrSrbrNYzZ?file=/&fileId=136606&x=1920&y=1080&a=true&etag=183eaedb94cd1a5685372f073900dc08"
3.  **Send Image**: The tool returns an image URL. Use the `message` tool to send it.
    *   `type`: "image"
    *   `media`: The URL returned by `media-generation`.
    *   `content`: A brief description.

**Example Tool Call Sequence:**

1.  Call `media-generation`:
    ```json
    {   
      "task": "image-to-image",
      "prompt": "make a pic of this person, but wearing sunglasses at a cafe. the person is taking a mirror selfie",
      "model": "Qwen/Qwen-Image-Edit-2509",
      "image_url": "https://share.cheersucloud.com/apps/files_sharing/publicpreview/7XYomPrSrbrNYzZ?file=/&fileId=136606&x=1920&y=1080&a=true&etag=183eaedb94cd1a5685372f073900dc08"
    }
    ```
2.  (Receive URL: "https://...")
3.  Call `message`:
    ```json
    {
      "type": "image",
      "media": "https://...",
      "content": "Here is a selfie of me at the cafe!"
    }
    ```
