using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Data;
using Network;
using UnityEngine;
using UnityEngine.UI;

public class ItemManager : MonoBehaviour
{
    [SerializeField] private GameObject _itemCellPrefab;
    [SerializeField] private RectTransform _contentRoot;

    [SerializeField] private Toggle _itemEquipToggle;
    [SerializeField] private Toggle _itemEnhanceToggle;
    [SerializeField] private Toggle _itemExpToggle;
    [SerializeField] private Toggle _itemTimerToggle;

    private UserCard[] _cards;
    private UserItem[] _exps;
    private UserItem[] _timers;
    private List<ItemCell> _cells = new ();

    public enum ItemTabType
    {
        Equip,
        Enhance,
        Exp,
        Timer,
    }

    private ItemTabType _tabType = ItemTabType.Equip;
    
    async void Start()
    {
        await RefreshAsync(ItemTabType.Equip);
        
        _itemEquipToggle.onValueChanged.AddListener((isOn) =>
        {
            if (isOn) RefreshAsync(ItemTabType.Equip);
        });
        _itemEnhanceToggle.onValueChanged.AddListener((isOn) =>
        {
            if (isOn) RefreshAsync(ItemTabType.Enhance);
        });
        _itemExpToggle.onValueChanged.AddListener((isOn) =>
        {
            if (isOn) RefreshAsync(ItemTabType.Exp);
        });
        _itemTimerToggle.onValueChanged.AddListener((isOn) =>
        {
            if (isOn) RefreshAsync(ItemTabType.Timer);
        });
    }

    private async Task RefreshAsync(ItemTabType type)
    {
        Debug.Log("Refresh");
        _tabType = type;
        
        for (int i = 0; i < _contentRoot.childCount; i++)
        {
            Destroy(_contentRoot.GetChild(i).gameObject);
        }
        
        var res = await GameManager.apiClient.ListItemAsync();
        _cards = res.cards;
        _exps = res.items.Where(i => i.itemType == (int)ItemType.Exp).ToArray();
        _timers = res.items.Where(i => i.itemType == (int)ItemType.Timer).ToArray();
        _cells.Clear();

        switch (_tabType)
        {
            case ItemTabType.Equip:
            case ItemTabType.Enhance:
                for (int i = 0; i < _cards.Length; i++)
                {
                    var card = _cards[i];
                    var cell = Instantiate(_itemCellPrefab, _contentRoot).GetComponent<ItemCell>();
                    cell.SetCard(card, () => OnEquipToggleChanged(), () => OnItemEnhance(card));
                    cell.SetType(_tabType);
                    _cells.Add(cell);
                }
                break;
            
            case ItemTabType.Exp:
                for (int i = 0; i < _exps.Length; i++)
                {
                    var cell = Instantiate(_itemCellPrefab, _contentRoot).GetComponent<ItemCell>();
                    cell.SetItem(_exps[i], (item) => { });
                    cell.SetType(_tabType);
                    _cells.Add(cell);
                }
                break;
            
            case ItemTabType.Timer:
                for (int i = 0; i < _timers.Length; i++)
                {
                    var cell = Instantiate(_itemCellPrefab, _contentRoot).GetComponent<ItemCell>();
                    cell.SetItem(_timers[i], OnUseTimer);
                    cell.SetType(_tabType);
                    _cells.Add(cell);
                }
                break;
        }
    }

    private async void OnEnhanceButtonPressed(UserCard card)
    {
        var item = _exps.FirstOrDefault(i => i.itemType == (int)ItemType.Exp);
        if (item == null)
        {
            Debug.LogWarning("Item is empty");
            return;
        }
        
        await GameManager.apiClient.AddExpToCardAsync(card.id, new [] {new AddExpToCardRequest.Item(item.id, 1)});
        await RefreshAsync(_tabType);
    }

    private async void OnEquipToggleChanged()
    {
        var equipCardIds = new long[3];
        var nextEquipIndex = 0;
        for (int i = 0; i < _cells.Count; i++)
        {
            if (_cells[i].IsEquipOn)
            {
                if (nextEquipIndex >= 3)
                {
                    Debug.LogWarning("Selected 3 or more cards to equip");
                    break;
                }
                
                equipCardIds[nextEquipIndex] = _cards[i].id;
                nextEquipIndex++;
            }
        }

        if (nextEquipIndex < 3)
        {
            Debug.LogWarning("Selected 2 or less cards to equip");
            return;
        }

        var res = await GameManager.apiClient.UpdateDeckAsync(equipCardIds);

        var deck = res.updatedResources.userDecks[0];
        for (int i = 0; i < _cards.Length; i++)
        {
            var card = _cards[i];
            if (card.id == deck.cardId1) GameManager.userData.deck.card1 = card;
            if (card.id == deck.cardId2) GameManager.userData.deck.card2 = card;
            if (card.id == deck.cardId3) GameManager.userData.deck.card3 = card;
        }

        var homeManager = GameObject.FindObjectOfType<HomeManager>();
        await homeManager.RefreshDeckAsync();
    }

    private void OnItemEnhance(UserCard card)
    {
        DialogManager.Instance.ShowEnhanceDialog(card);
    }

    private void OnUseTimer(UserItem item)
    {
        DialogManager.Instance.ShowMessageDialog("タイマー使用エラー",
            "APIがないため残念ながら未実装です。\nゲームを遊んで頂きありがとうございます。");
    }
}
