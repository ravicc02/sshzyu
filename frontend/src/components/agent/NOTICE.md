# Conversation renderer

The Agent uses the platform Vue components and its existing `marked` (MIT) and `DOMPurify` (Apache-2.0 OR MPL-2.0) dependencies for sanitized Markdown. It no longer mounts Deep Chat; no external chat service, provider key or remote media loader is used. Model calls, private images and billing remain in the platform backend.
