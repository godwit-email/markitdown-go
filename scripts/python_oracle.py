import os
import sys
from io import BytesIO

from markitdown import MarkItDown, StreamInfo


def extension(filename: str) -> str | None:
    name = filename.replace("\\", "/").split("/")[-1]
    ext = os.path.splitext(name)[1].lower()
    return ext or None


def main() -> None:
    media_type, filename, charset = sys.argv[1], sys.argv[2], sys.argv[3]
    data = sys.stdin.buffer.read()
    info = StreamInfo(
        mimetype=media_type or None,
        extension=extension(filename),
        charset=charset or None,
        filename=filename or None,
    )
    result = MarkItDown().convert_stream(BytesIO(data), stream_info=info)
    sys.stdout.buffer.write(result.markdown.encode("utf-8"))


if __name__ == "__main__":
    main()
