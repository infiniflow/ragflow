use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "kb_info")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub kb_id: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub uid: i64,
    pub kn_name: String,
    pub icon: i64,

    pub created_at: DateTimeWithTimeZone,
    pub updated_at: DateTimeWithTimeZone,
    #[sea_orm(soft_delete_column)]
    pub is_deleted: bool,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::doc_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::kb_2_doc::Relation::DocInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::kb_2_doc::Relation::KbInfo.def().rev())
    }
}

impl Related<super::dialog_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::dialog_2_kb::Relation::DialogInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::dialog_2_kb::Relation::KbInfo.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}