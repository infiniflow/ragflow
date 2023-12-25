use sea_orm::entity::prelude::*;
use serde::{ Deserialize, Serialize };
use chrono::{ DateTime, FixedOffset };

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "kb_info")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    #[serde(skip_deserializing)]
    pub kb_id: i64,
    #[sea_orm(index)]
    pub uid: i64,
    pub kb_name: String,
    pub icon: i16,

    #[serde(skip_deserializing)]
    pub created_at: DateTime<FixedOffset>,
    #[serde(skip_deserializing)]
    pub updated_at: DateTime<FixedOffset>,
    #[serde(skip_deserializing)]
    pub is_deleted: bool,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::doc_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::kb2_doc::Relation::DocInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::kb2_doc::Relation::KbInfo.def().rev())
    }
}

impl Related<super::dialog_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::dialog2_kb::Relation::DialogInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::dialog2_kb::Relation::KbInfo.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}
