use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, Hash, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "user_info")]
pub struct Model {
    #[sea_orm(primary_key)]
    #[serde(skip_deserializing)]
    pub uid: i64,
    pub email: String,
    pub nickname: String,
    pub avatar_url: Option<String>,
    pub color_schema: String,
    pub list_style: String,
    pub language: String,
    pub password: String,

    #[serde(skip_deserializing)]
    pub created_at: Date,
    #[serde(skip_deserializing)]
    pub updated_at: Date,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl ActiveModelBehavior for ActiveModel {}