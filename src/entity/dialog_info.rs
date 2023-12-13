use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "dialog_info")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub dialog_id: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub uid: i64,
    pub dialog_name: String,
    pub history: String,

    pub created_at: DateTimeWithTimeZone,
    pub updated_at: DateTimeWithTimeZone,
    #[sea_orm(soft_delete_column)]
    pub is_deleted: bool,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {}

impl Related<super::kb_info::Entity> for Entity {
    fn to() -> RelationDef {
        super::dialog_2_kb::Relation::KbInfo.def()
    }

    fn via() -> Option<RelationDef> {
        Some(super::dialog_2_kb::Relation::DialogInfo.def().rev())
    }
}

impl ActiveModelBehavior for ActiveModel {}