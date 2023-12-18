use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};
use crate::entity::kb_info;

#[derive(Clone, Debug, PartialEq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "doc_info")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub did: i64,
    #[sea_orm(index)]
    pub uid: i64,
    pub doc_name: String,
    pub size: u64,
    #[sea_orm(column_name = "type")]
    pub r#type: String,
    pub kb_progress: f64,
    pub kb_progress_msg: String,
    pub location: String,
    #[sea_orm(ignore)]
    pub kb_infos: Vec<kb_info::Model>,

    #[serde(skip_deserializing)]
    pub created_at: Date,
    #[serde(skip_deserializing)]
    pub updated_at: Date,
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

impl Related<Entity> for Entity {
    fn to() -> RelationDef {
        super::doc2_doc::Relation::Parent.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::doc2_doc::Relation::Child.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}
