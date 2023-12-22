use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};
use chrono::{DateTime, FixedOffset};

#[derive(Clone, Debug, PartialEq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "tag_info")]
pub struct Model {
    #[sea_orm(primary_key)]
    #[serde(skip_deserializing)]
    pub tid: i64,
    #[sea_orm(index)]
    pub uid: i64,
    pub tag_name: String,
    #[serde(skip_deserializing)]
    pub regx: String,
    pub color: i16,
    pub icon: i16,
    #[serde(skip_deserializing)]
    pub folder_id: i64,

    #[serde(skip_deserializing)]
    pub created_at: DateTime<FixedOffset>,
    #[serde(skip_deserializing)]
    pub updated_at: DateTime<FixedOffset>,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::doc_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::tag2_doc::Relation::DocInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::tag2_doc::Relation::Tag.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}