use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "doc_info")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub did: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub uid: i64,
    pub doc_name: String,
    pub size: i64,
    #[sea_orm(column_name = "type")]
    pub r#type: String,
    pub kb_progress: f64,

    pub created_at: DateTimeWithTimeZone,
    pub updated_at: DateTimeWithTimeZone,
    #[sea_orm(soft_delete_column)]
    pub is_deleted: bool,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::tag_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::tag_2_doc::Relation::Tag.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::tag_2_doc::Relation::DocInfo.def().rev())
    }
}

impl Related<super::kb_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::kb_2_doc::Relation::KbInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::kb_2_doc::Relation::DocInfo.def().rev())
    }
}

impl Related<Entity> for Entity {
    fn to() -> RelationDef {
        super::doc_2_doc::Relation::Parent.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::doc_2_doc::Relation::Child.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}