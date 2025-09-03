from pydantic import BaseModel, ConfigDict, Field


class ParserFromUpstream(BaseModel):
    created_time: float | None = Field(default=None, alias="_created_time")
    elapsed_time: float | None = Field(default=None, alias="_elapsed_time")

    name: str
    blob: bytes

    model_config = ConfigDict(populate_by_name=True, extra="forbid")
