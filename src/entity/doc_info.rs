use sea_orm::entity::prelude::*;
use serde::{ Deserialize, Serialize };
use crate::entity::kb_info;
use chrono::{ DateTime, FixedOffset };

#[derive(Clone, Debug, PartialEq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "doc_info")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub did: i64,
    #[sea_orm(index)]
    pub uid: i64,
    pub doc_name: String,
    pub size: i64,
    #[sea_orm(column_name = "type")]
    pub r#type: String,
    #[serde(skip_deserializing)]
    pub location: String,
    #[serde(skip_deserializing)]
    pub created_at: DateTime<FixedOffset>,
    #[serde(skip_deserializing)]
    pub updated_at: DateTime<FixedOffset>,
    #[serde(skip_deserializing)]
    pub is_deleted: bool,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::tag_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::tag2_doc::Relation::Tag.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::tag2_doc::Relation::DocInfo.def().rev())
    }
}

impl Related<super::kb_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::kb2_doc::Relation::KbInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::kb2_doc::Relation::DocInfo.def().rev())
    }
}

impl Related<super::doc2_doc::Entity> for Entity {
    fn to() -> RelationDef {
        super::doc2_doc::Relation::Parent.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::doc2_doc::Relation::Child.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}
