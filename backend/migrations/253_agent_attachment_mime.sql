-- Standard OOXML MIME names exceed the original image-only field length.
ALTER TABLE agent_assets ALTER COLUMN mime_type TYPE varchar(128);
