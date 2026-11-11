"""HTTP exceptions used by the API.

Defined locally (previously re-exported from fastcrud, which pulled in SQLAlchemy
and friends for a service that has no database).
"""

from http import HTTPStatus

from fastapi import HTTPException, status


class CustomException(HTTPException):
    def __init__(self, status_code: int = status.HTTP_500_INTERNAL_SERVER_ERROR, detail: str | None = None) -> None:
        if detail is None:
            detail = HTTPStatus(status_code).description
        super().__init__(status_code=status_code, detail=detail)


class BadRequestException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_400_BAD_REQUEST, detail)


class UnauthorizedException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_401_UNAUTHORIZED, detail)


class ForbiddenException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_403_FORBIDDEN, detail)


class NotFoundException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_404_NOT_FOUND, detail)


class DuplicateValueException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_409_CONFLICT, detail)


class UnprocessableEntityException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_422_UNPROCESSABLE_CONTENT, detail)


class RateLimitException(CustomException):
    def __init__(self, detail: str | None = None) -> None:
        super().__init__(status.HTTP_429_TOO_MANY_REQUESTS, detail)
