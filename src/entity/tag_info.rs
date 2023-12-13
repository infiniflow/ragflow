use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "tag_info")]
pub struct Model {
    #[sea_orm(primary_key)]
    #[serde(skip_deserializing)]
    pub tid: i64,
    pub uid: i64,
    pub tag_name: String,
    pub regx: String,
    pub color: i64,
    pub icon: i64,
    pub dir: String,

    #[serde(skip_deserializing)]
    pub created_at: Date,
    #[serde(skip_deserializing)]
    pub updated_at: Date,
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